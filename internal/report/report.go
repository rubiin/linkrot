package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"linkrot/internal/model"
)

const (
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiOrange = "\x1b[38;5;208m"
	ansiCyan   = "\x1b[36m"
	ansiGray   = "\x1b[37m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
	ansiReset  = "\x1b[0m"
)

type Options struct {
	JSON       bool
	Summary    bool
	Color      bool
	FileCount  int  // number of files scanned
	ScannedDir bool // whether input was a directory (show "Found X in Y files" format)
}

func PrintResults(results []model.LinkCheck, opts Options) {
	if opts.JSON {
		printJSON(results)
		return
	}
	printText(results, opts)
}

func colorize(s, code string, enabled bool) string {
	if !enabled {
		return s
	}
	return code + s + ansiReset
}

// ShouldColorize resolves the --color mode against the environment:
// never wins, NO_COLOR wins over everything (even always), auto defers to TTY.
func ShouldColorize(mode string, isTTY, noColor bool) bool {
	switch mode {
	case "always":
		return !noColor
	case "auto":
		return isTTY && !noColor
	default: // "never" and anything unrecognized
		return false
	}
}

func printText(results []model.LinkCheck, opts Options) {
	if len(results) == 0 {
		fmt.Println("No links found.")
		return
	}

	if opts.Summary {
		fmt.Println()
		if opts.ScannedDir && opts.FileCount > 0 {
			fmt.Println(colorize(fmt.Sprintf("Found %d link(s) in %d file(s)", len(results), opts.FileCount), ansiGreen, opts.Color))
		} else {
			fmt.Println(colorize("Links found: "+strconv.Itoa(len(results)), ansiGreen, opts.Color))
		}
		fmt.Println()
	}

	for _, r := range sortedDeadFirst(results) {
		status := ""
		if r.Alive {
			status = fmt.Sprintf(" → %s", colorize(strconv.Itoa(r.Status), ansiGreen, opts.Color))
		} else {
			marker := "DEAD (err)"
			if r.Status != 0 {
				marker = fmt.Sprintf("DEAD (%d)", r.Status)
			}
			status = fmt.Sprintf(" → %s", colorize(marker, ansiRed, opts.Color))
		}

		url := colorize(truncateURL(r.URL), ansiCyan, opts.Color)
		fmt.Println(url + status)

		// Errors are omitted from text output to keep it easy to scan.
		_ = r.Err
		if len(r.RedirectChain) > 0 {
			chain := make([]string, len(r.RedirectChain))
			for i, step := range r.RedirectChain {
				chain[i] = strconv.Itoa(step.Status) + " " + truncateURL(step.URL)
			}
			fmt.Printf("  redirect chain: %s\n", strings.Join(chain, " -> "))
		}
	}

	if opts.Summary {
		alive := 0
		for _, r := range results {
			if r.Alive {
				alive++
			}
		}
		dead := len(results) - alive
		fmt.Println("")
		fmt.Println("Summary: " + colorize(strconv.Itoa(alive)+" alive", ansiGreen, opts.Color) +
			", " + colorize(strconv.Itoa(dead)+" dead", ansiRed, opts.Color))
	}
}

func printJSON(results []model.LinkCheck) {
	type redirectJSON struct {
		URL    string `json:"url"`
		Status int    `json:"status"`
	}
	type jsonResult struct {
		URL           string         `json:"url"`
		Status        int            `json:"status"`
		Alive         bool           `json:"alive"`
		RedirectChain []redirectJSON `json:"redirect_chain,omitempty"`
		Err           string         `json:"error,omitempty"`
		SourceFile    string         `json:"source_file,omitempty"`
	}

	out := make([]jsonResult, 0, len(results))
	for _, r := range sortedDeadFirst(results) {
		jr := jsonResult{
			URL:        r.URL,
			Status:     r.Status,
			Alive:      r.Alive,
			Err:        r.Err,
			SourceFile: r.SourceFile,
		}
		for _, step := range r.RedirectChain {
			jr.RedirectChain = append(jr.RedirectChain, redirectJSON{URL: step.URL, Status: step.Status})
		}
		out = append(out, jr)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling JSON: %s\n", err)
		return
	}
	fmt.Println(string(data))
}

func truncateURL(url string) string {
	if len(url) <= 90 {
		return url
	}
	return url[:87] + "..."
}

func sortedDeadFirst(results []model.LinkCheck) []model.LinkCheck {
	sorted := make([]model.LinkCheck, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Alive != sorted[j].Alive {
			return !sorted[i].Alive
		}
		return sorted[i].URL < sorted[j].URL
	})
	return sorted
}
