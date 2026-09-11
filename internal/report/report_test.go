package report

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"linkrot/internal/model"
)

func TestColorize(t *testing.T) {
	if got := colorize("x", ansiRed, true); got != "\x1b[31mx\x1b[0m" {
		t.Errorf("colorize enabled: got %q", got)
	}
	if got := colorize("x", ansiRed, false); got != "x" {
		t.Errorf("colorize disabled: got %q", got)
	}
}

func TestShouldColorize(t *testing.T) {
	tests := []struct {
		mode    string
		isTTY   bool
		noColor bool
		want    bool
	}{
		{"never", true, false, false},
		{"always", false, false, true},
		{"always", false, true, false}, // NO_COLOR beats even --color=always
		{"auto", true, false, true},
		{"auto", false, false, false},
		{"auto", true, true, false},
		{"bogus", true, false, false},
	}
	for _, tt := range tests {
		if got := ShouldColorize(tt.mode, tt.isTTY, tt.noColor); got != tt.want {
			t.Errorf("shouldColorize(%q, %v, %v) = %v, want %v", tt.mode, tt.isTTY, tt.noColor, got, tt.want)
		}
	}
}

// captureStdout runs fn while capturing os.Stdout, returning the output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPrintTextColor(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
		{URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
	}

	colored := captureStdout(t, func() { PrintResults(results, Options{Color: true}) })
	if !strings.Contains(colored, "\x1b[31m") || !strings.Contains(colored, "\x1b[32m") {
		t.Errorf("expected red and green escapes in colored output, got: %q", colored)
	}

	plain := captureStdout(t, func() { PrintResults(results, Options{}) })
	if strings.Contains(plain, "\x1b[") {
		t.Errorf("expected no escapes in plain output, got: %q", plain)
	}
}

func TestPrintText(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
		{URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{Summary: false}) })

	if !strings.Contains(output, "https://example.com/dead") {
		t.Errorf("expected dead link in output, got: %s", output)
	}
	if !strings.Contains(output, "https://example.com/ok") {
		t.Errorf("expected alive link in output, got: %s", output)
	}
	deadIdx := strings.Index(output, "https://example.com/dead")
	okIdx := strings.Index(output, "https://example.com/ok")
	if deadIdx > okIdx {
		t.Errorf("expected dead link before alive link in output:\n%s", output)
	}
}

func TestPrintTextWithSummary(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
		{URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
		{URL: "https://example.com/also-dead", Status: 0, Alive: false, Err: "connection refused", SourceFile: "test.html"},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{Summary: true}) })

	if !strings.Contains(output, "Summary: 1 alive, 2 dead") {
		t.Errorf("expected summary line 'Summary: 1 alive, 2 dead', got: %s", output)
	}
	// Summary must be the last line
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if !strings.HasPrefix(lines[len(lines)-1], "Summary:") {
		t.Errorf("expected summary as last line, got: %q", lines[len(lines)-1])
	}
}

func TestPrintTextSummaryDisabled(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{Summary: false}) })

	if strings.Contains(output, "Summary:") {
		t.Errorf("expected no summary line, got: %s", output)
	}
}

func TestPrintTextSummaryWithNoLinks(t *testing.T) {
	output := captureStdout(t, func() { PrintResults(nil, Options{Summary: true}) })
	if !strings.Contains(output, "No links found") {
		t.Errorf("expected 'No links found' message, got: %s", output)
	}
	if strings.Contains(output, "Summary:") {
		t.Errorf("expected no summary line when there are no links, got: %s", output)
	}
}

func TestPrintTextEmpty(t *testing.T) {
	output := captureStdout(t, func() { PrintResults(nil, Options{}) })
	if !strings.Contains(output, "No links found") {
		t.Errorf("expected 'No links found' message, got: %s", output)
	}
}

func TestPrintTextWithError(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://unreachable.test/x", Status: 0, Alive: false, Err: "connection refused", SourceFile: "a.html"},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{}) })

	// Error details are intentionally omitted from text output to keep it easy to scan;
	// the dead link itself should still appear.
	if !strings.Contains(output, "https://unreachable.test/x") {
		t.Errorf("expected URL in output, got: %s", output)
	}
}

func TestPrintTextWithRedirectChain(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/loop", Status: 404, Alive: false, SourceFile: "a.html",
			RedirectChain: []model.RedirectStep{
				{URL: "https://example.com/loop", Status: 302},
				{URL: "https://example.com/moved", Status: 301},
			}},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{}) })

	if !strings.Contains(output, "redirect") {
		t.Errorf("expected redirect info in output, got: %s", output)
	}
	if !strings.Contains(output, "302 https://example.com/loop") {
		t.Errorf("expected first hop in output, got: %s", output)
	}
}

func TestPrintJSON(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
		{URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{JSON: true}) })

	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %s", output)
	}

	if len(parsed) != 2 {
		t.Errorf("expected 2 JSON results, got %d", len(parsed))
	}

	foundDead := false
	for _, item := range parsed {
		if item["url"] == "https://example.com/dead" {
			foundDead = true
			if item["alive"] != false {
				t.Errorf("expected dead link to have alive=false")
			}
			if item["status"] != float64(404) {
				t.Errorf("expected dead link status 404, got %v", item["status"])
			}
		}
	}
	if !foundDead {
		t.Error("dead link not found in JSON output")
	}
}

func TestPrintJSONWithRedirectChain(t *testing.T) {
	results := []model.LinkCheck{
		{URL: "https://example.com/loop", Status: 404, Alive: false, SourceFile: "a.html",
			RedirectChain: []model.RedirectStep{
				{URL: "https://example.com/loop", Status: 302},
			}},
	}

	output := captureStdout(t, func() { PrintResults(results, Options{JSON: true}) })

	var parsed []struct {
		URL           string `json:"url"`
		RedirectChain []struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		} `json:"redirect_chain"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %s", output)
	}
	if len(parsed) != 1 || len(parsed[0].RedirectChain) != 1 {
		t.Fatalf("expected 1 result with 1 redirect step, got: %s", output)
	}
	if parsed[0].RedirectChain[0].Status != 302 {
		t.Errorf("expected redirect status 302, got %d", parsed[0].RedirectChain[0].Status)
	}
}
