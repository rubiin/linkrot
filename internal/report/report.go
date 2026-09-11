package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"linkrot/internal/model"
)

// PrintResults prints the check results to stdout. Dead links are reported
// first. If jsonOutput is true, results are printed as a JSON array.
func PrintResults(results []model.LinkCheck, jsonOutput bool) {
	if jsonOutput {
		printJSON(results)
		return
	}
	printText(results)
}

func printText(results []model.LinkCheck) {
	if len(results) == 0 {
		fmt.Println("No links found.")
		return
	}

	sorted := sortedDeadFirst(results)

	for _, r := range sorted {
		if r.Alive {
			fmt.Printf("%s → %d\n", r.URL, r.Status)
			continue
		}

		var msg string
		if r.Status == 0 {
			msg = fmt.Sprintf("%s → DEAD (err)", r.URL)
		} else {
			msg = fmt.Sprintf("%s → DEAD (%d)", r.URL, r.Status)
		}
		if r.Err != "" {
			msg += fmt.Sprintf(" %s", r.Err)
		}
		if len(r.RedirectChain) > 0 {
			chain := make([]string, len(r.RedirectChain))
			for i, step := range r.RedirectChain {
				chain[i] = fmt.Sprintf("%d %s", step.Status, step.URL)
			}
			msg += fmt.Sprintf(" [redirect: %s]", strings.Join(chain, " -> "))
		}
		fmt.Println(msg)
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

// sortedDeadFirst returns a copy of results sorted dead links first, then by URL.
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
