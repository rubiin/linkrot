package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	term "github.com/mattn/go-isatty"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"linkrot/internal/checker"
	"linkrot/internal/report"
)

type CheckConfig struct {
	Root                string        `yaml:"root"`
	Threads             int           `yaml:"threads"`
	Timeout             time.Duration `yaml:"timeout"`
	CacheTTL            time.Duration `yaml:"cache-ttl"`
	Retry               int           `yaml:"retry"`
	UserAgent           string        `yaml:"user-agent"`
	AllowFileExtensions []string      `yaml:"allow-file-extensions"`
	IgnoreHosts         []string      `yaml:"ignore-hosts"`
	IgnoreFiles         []string      `yaml:"ignore-files"`
	JSONOutput          bool          `yaml:"json"`
	Summary             *bool         `yaml:"summary"`
	Color               string        `yaml:"color"`
	ConfigFile          string        `yaml:"-"`
	Files               []string      `yaml:"-"`
}

// summaryFlag backs --summary; loadConfigFile folds it into checkConfig.
var summaryFlag bool

// colorFlag backs --color, same folding as summaryFlag.
var colorFlag string

var checkConfig = CheckConfig{
	Threads:   10,
	Timeout:   10 * time.Second,
	CacheTTL:  259200 * time.Second,
	Retry:     2,
	UserAgent: "linkrot/0.1.0",
	Summary:   boolPtr(true),
	Color:     "auto",
}

var checkCmd = &cobra.Command{
	Use:   "check <files...>",
	Short: "Check links in local files",
	Long:  "linkrot check reads links from the given files, resolves them relative to --root, and checks each URL over HTTP.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runCheck,
}

func init() {
	checkCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return loadConfigFile(cmd)
	}

	checkCmd.Flags().StringVar(&checkConfig.Root, "root", ".", "root directory for resolving relative links")
	checkCmd.Flags().IntVarP(&checkConfig.Threads, "threads", "n", 10, "number of concurrent HTTP checks")
	checkCmd.Flags().DurationVarP(&checkConfig.Timeout, "timeout", "t", 10*time.Second, "per-request timeout")
	checkCmd.Flags().DurationVar(&checkConfig.CacheTTL, "cache-ttl", 259200*time.Second, "cache TTL for checked URLs")
	checkCmd.Flags().IntVar(&checkConfig.Retry, "retry", 2, "number of retries on 502/503/504")
	checkCmd.Flags().StringVar(&checkConfig.UserAgent, "user-agent", "linkrot/0.1.0", "User-Agent header")
	checkCmd.Flags().StringSliceVar(&checkConfig.AllowFileExtensions, "allow-file-extensions", nil, "only parse files with these extensions (comma-separated)")
	checkCmd.Flags().StringSliceVar(&checkConfig.IgnoreHosts, "ignore-hosts", nil, "skip URLs whose host is in this list (comma-separated)")
	checkCmd.Flags().StringSliceVar(&checkConfig.IgnoreFiles, "ignore-files", nil, "ignore patterns from these files (e.g. .gitignore,.linkrotignore); missing files are silently skipped")
	checkCmd.Flags().BoolVar(&checkConfig.JSONOutput, "json", false, "output as JSON array")
	checkCmd.Flags().BoolVar(&summaryFlag, "summary", true, "append a summary line (X alive, Y dead) to text output")
	checkCmd.Flags().StringVar(&colorFlag, "color", "auto", "when to colorize output: auto, always, never")
	checkCmd.Flags().StringVarP(&checkConfig.ConfigFile, "config", "c", "", "path to YAML config file (default: XDG config dir)")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	checkConfig.Files = args

	cfg := checker.CheckConfig{
		Root:                checkConfig.Root,
		Threads:             checkConfig.Threads,
		Timeout:             checkConfig.Timeout,
		CacheTTL:            checkConfig.CacheTTL,
		Retry:               checkConfig.Retry,
		UserAgent:           checkConfig.UserAgent,
		AllowFileExtensions: checkConfig.AllowFileExtensions,
		IgnoreHosts:         checkConfig.IgnoreHosts,
		IgnoreFiles:         checkConfig.IgnoreFiles,
	}

	useSpinner := !checkConfig.JSONOutput && colorEnabled()

	var sp *spinner.Spinner
	if useSpinner {
		sp = spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		sp.Suffix = " Checking links..."
		sp.Start()
	}

	results := checker.CheckAll(cmd.Context(), cfg, checkConfig.Files)
	fileCount := checker.FileCount()
	scannedDir := checker.ScannedDirectory()

	if sp != nil {
		sp.Stop()
	}

	report.PrintResults(results, report.Options{
		JSON:      checkConfig.JSONOutput,
		Summary:   summaryEnabled(),
		Color:     colorEnabled(),
		FileCount: fileCount,
		ScannedDir: scannedDir,
	})

	for _, r := range results {
		if !r.Alive {
			os.Exit(1)
		}
	}
	return nil
}

// loadConfigFile applies config file values on top of the parsed flags.
// Anything set explicitly on the command line wins.
func loadConfigFile(cmd *cobra.Command) error {
	checkConfig.Summary = &summaryFlag
	checkConfig.Color = colorFlag

	cfgPath := checkConfig.ConfigFile
	if cfgPath == "" {
		cfgPath = defaultConfigPath()
	}
	if cfgPath == "" {
		return nil
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}

	var fileCfg CheckConfig
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("parsing config file %s: %w", cfgPath, err)
	}

	var flags *pflag.FlagSet
	if cmd != nil {
		flags = cmd.Flags()
	}
	changed := func(name string) bool {
		return flags != nil && flags.Changed(name)
	}

	if !changed("root") && fileCfg.Root != "" {
		checkConfig.Root = fileCfg.Root
	}
	if !changed("threads") && fileCfg.Threads != 0 {
		checkConfig.Threads = fileCfg.Threads
	}
	if !changed("timeout") && fileCfg.Timeout != 0 {
		checkConfig.Timeout = fileCfg.Timeout
	}
	if !changed("cache-ttl") && fileCfg.CacheTTL != 0 {
		checkConfig.CacheTTL = fileCfg.CacheTTL
	}
	if !changed("retry") && fileCfg.Retry != 0 {
		checkConfig.Retry = fileCfg.Retry
	}
	if !changed("user-agent") && fileCfg.UserAgent != "" {
		checkConfig.UserAgent = fileCfg.UserAgent
	}
	if !changed("allow-file-extensions") && len(fileCfg.AllowFileExtensions) > 0 {
		checkConfig.AllowFileExtensions = fileCfg.AllowFileExtensions
	}
	if !changed("ignore-hosts") && len(fileCfg.IgnoreHosts) > 0 {
		checkConfig.IgnoreHosts = fileCfg.IgnoreHosts
	}
	if !changed("ignore-files") && len(fileCfg.IgnoreFiles) > 0 {
		checkConfig.IgnoreFiles = fileCfg.IgnoreFiles
	}
	if !changed("json") && fileCfg.JSONOutput {
		checkConfig.JSONOutput = true
	}
	if !changed("summary") && fileCfg.Summary != nil {
		checkConfig.Summary = fileCfg.Summary
	}
	if !changed("color") && fileCfg.Color != "" {
		checkConfig.Color = fileCfg.Color
	}

	return nil
}

func summaryEnabled() bool {
	return checkConfig.Summary == nil || *checkConfig.Summary
}

// colorEnabled resolves the color mode against the terminal.
func colorEnabled() bool {
	return report.ShouldColorize(checkConfig.Color, term.IsTerminal(os.Stdout.Fd()), os.Getenv("NO_COLOR") != "")
}

func boolPtr(b bool) *bool {
	return &b
}

func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "linkrot", "config.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "linkrot", "config.yaml")
}
