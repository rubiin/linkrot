package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newTestFlagSet builds a flagset mirroring checkCmd's flags with none of them
// marked as changed (i.e. no flag was explicitly set on the command line).
func newTestFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("root", ".", "")
	fs.Int("threads", 10, "")
	fs.Duration("timeout", 10*time.Second, "")
	fs.Duration("cache-ttl", 259200*time.Second, "")
	fs.Int("retry", 2, "")
	fs.String("user-agent", "linkrot/0.1.0", "")
	fs.StringSlice("allow-file-extensions", nil, "")
	fs.StringSlice("ignore-hosts", nil, "")
	fs.Bool("json", false, "")
	fs.String("config", "", "")
	return fs
}

func TestLoadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`root: "/custom/root"
threads: 5
timeout: 5s
cache-ttl: 3600s
retry: 3
user-agent: "custom-agent"
allow-file-extensions:
  - ".html"
  - ".md"
ignore-hosts:
  - "example.com"
json: true
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Reset checkConfig to defaults
	checkConfig = CheckConfig{
		Threads:     10,
		Timeout:     10 * time.Second,
		CacheTTL:    259200 * time.Second,
		Retry:       2,
		UserAgent:   "linkrot/0.1.0",
	}
	checkConfig.ConfigFile = configFile

	cmd := &cobra.Command{Use: "check"}
	cmd.Flags().AddFlagSet(newTestFlagSet())

	err = loadConfigFile(cmd)
	if err != nil {
		t.Fatalf("loadConfigFile returned error: %v", err)
	}

	if checkConfig.Root != "/custom/root" {
		t.Errorf("expected root /custom/root, got %s", checkConfig.Root)
	}
	if checkConfig.Threads != 5 {
		t.Errorf("expected threads 5, got %d", checkConfig.Threads)
	}
	if checkConfig.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", checkConfig.Timeout)
	}
	if checkConfig.CacheTTL != 3600*time.Second {
		t.Errorf("expected cache-ttl 3600s, got %v", checkConfig.CacheTTL)
	}
	if checkConfig.Retry != 3 {
		t.Errorf("expected retry 3, got %d", checkConfig.Retry)
	}
	if checkConfig.UserAgent != "custom-agent" {
		t.Errorf("expected user-agent custom-agent, got %s", checkConfig.UserAgent)
	}
	if len(checkConfig.AllowFileExtensions) != 2 {
		t.Errorf("expected 2 allow-file-extensions, got %d", len(checkConfig.AllowFileExtensions))
	}
	if len(checkConfig.IgnoreHosts) != 1 {
		t.Errorf("expected 1 ignore-host, got %d", len(checkConfig.IgnoreHosts))
	}
	if !checkConfig.JSONOutput {
		t.Error("expected json true, got false")
	}
}

func TestLoadConfigFileOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configFile, []byte(`threads: 5
json: true
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	checkConfig = CheckConfig{
		Threads:     10,
		Timeout:     10 * time.Second,
		CacheTTL:    259200 * time.Second,
		Retry:       2,
		UserAgent:   "linkrot/0.1.0",
		JSONOutput:  false,
	}
	checkConfig.ConfigFile = configFile

	cmd := &cobra.Command{Use: "check"}
	fs := newTestFlagSet()
	cmd.Flags().AddFlagSet(fs)
	// Simulate the user setting --threads 20 on the command line: cobra's flag
	// parsing writes the value into the bound var and marks the flag changed.
	if err := fs.Set("threads", "20"); err != nil {
		t.Fatal(err)
	}
	checkConfig.Threads = 20
	// Simulate --json=false explicitly: flag wins over config's json: true.
	if err := fs.Set("json", "false"); err != nil {
		t.Fatal(err)
	}
	checkConfig.JSONOutput = false

	err = loadConfigFile(cmd)
	if err != nil {
		t.Fatalf("loadConfigFile returned error: %v", err)
	}

	if checkConfig.Threads != 20 {
		t.Errorf("expected threads 20 (flag override), got %d", checkConfig.Threads)
	}
	if checkConfig.JSONOutput != false {
		t.Errorf("expected json false (flag --json=false overrides config json: true), got %v", checkConfig.JSONOutput)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	os.Unsetenv("XDG_CONFIG_HOME")
	path := defaultConfigPath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "linkrot", "config.yaml")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}

	os.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	path = defaultConfigPath()
	expected = filepath.Join("/custom/xdg", "linkrot", "config.yaml")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}
