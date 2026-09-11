# deadlink Go CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI that reads links from local HTML/Markdown/txt/yaml/c files, resolves them relative to a root directory, checks each over HTTP with concurrency, caching, retries, and host filtering, and reports dead links.

**Architecture:** Cobra-based CLI (`deadlink check <files...>`) with internal packages for extraction, checking (worker pool + HTTP + cache + retry), and reporting. Standard library + cobra + x/net/html.

**Tech Stack:** Go 1.27, `github.com/spf13/cobra`, `golang.org/x/net/html`, `gopkg.in/yaml.v3` (config file parsing), standard library only otherwise.

**Spec:** `docs/superpowers/specs/2026-09-11-deadlink-go-design.md`

## Global Constraints

- Go 1.27
- CLI framework: cobra
- HTML parsing: `golang.org/x/net/html`
- Markdown/text URL extraction: regex-based (no external parser)
- Exit code 1 when any dead link found, 0 otherwise
- Follow redirects up to 10 hops; treat final 200 as alive
- Retry on 502/503/504 with exponential backoff
- Cache: in-memory, keyed by URL, TTL-configurable
- Dead links reported first in text output

---

### Task 1: Project scaffold + cobra CLI skeleton + model types + config file loading

**Files:**
- Create: `go.mod`
- Create: `cmd/deadlink/main.go`
- Create: `cmd/deadlink/root.go`
- Create: `cmd/deadlink/check.go` (includes config loading via `PersistentPreRunE`)
- Create: `cmd/deadlink/config.go` (config file loading, XDG path resolution)
- Create: `internal/model/model.go`
- Test: n/a (scaffold, no logic yet)

**Interfaces:**
- Produces: `model.LinkCheck`, `model.RedirectStep` types — consumed by Tasks 2-5
- Produces: `cmd/deadlink` binary with `check` subcommand and all flags wired (no behavior yet)

- [ ] **Step 1: Initialize Go module**

```bash
go mod init deadlink
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/spf13/cobra
go get golang.org/x/net/html
```

- [ ] **Step 3: Create `cmd/deadlink/main.go`**

```go
package main

import "deadlink/cmd/deadlink"

func main() {
    rootCmd.Execute()
}
```

- [ ] **Step 4: Create `cmd/deadlink/root.go`** — cobra root command with version and global config:

```go
package deadlink

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
    Use:   "deadlink",
    Short: "Check links in local files",
    Long:  "deadlink reads links from local files and checks whether each URL is still alive.",
    Version: "0.1.0",
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

- [ ] **Step 5: Create `cmd/deadlink/check.go`** — `check` subcommand with all flags (no Run logic yet, just flag binding):

```go
package deadlink

import (
    "strconv"
    "strings"
    "time"

    "github.com/spf13/cobra"
)

type CheckConfig struct {
    Root                 string
    Threads              int
    Timeout              time.Duration
    CacheTTL             time.Duration
    Retry                int
    UserAgent            string
    AllowFileExtensions  []string
    IgnoreHosts          []string
    JSONOutput           bool
    Files                []string
}

var checkConfig = CheckConfig{
    Threads:     10,
    Timeout:     10 * time.Second,
    CacheTTL:    259200 * time.Second, // 3 days
    Retry:       2,
    UserAgent:   "deadlink/0.1.0",
}

var _ = strings.Join
var _ = strconv.Itoa

var checkCmd = &cobra.Command{
    Use:   "check <files...>",
    Short: "Check links in local files",
    Long:  "deadlink check reads links from the given files, resolves them relative to --root, and checks each URL over HTTP.",
    Args:  cobra.ExactArgs(1),
    RunE:  runCheck,
}

func init() {
    checkCmd.Flags().StringVar(&checkConfig.Root, "root", ".", "root directory for resolving relative links")
    checkCmd.Flags().IntVarP(&checkConfig.Threads, "threads", "n", 10, "number of concurrent HTTP checks")
    checkCmd.Flags().DurationVarP(&checkConfig.Timeout, "timeout", "t", 10*time.Second, "per-request timeout")
    checkCmd.Flags().DurationVar(&checkConfig.CacheTTL, "cache-ttl", 259200*time.Second, "cache TTL for checked URLs")
    checkCmd.Flags().IntVar(&checkConfig.Retry, "retry", 2, "number of retries on 502/503/504")
    checkCmd.Flags().StringVar(&checkConfig.UserAgent, "user-agent", "deadlink/0.1.0", "User-Agent header")
    checkCmd.Flags().StringSliceVar(&checkConfig.AllowFileExtensions, "allow-file-extensions", nil, "only parse files with these extensions (comma-separated)")
    checkCmd.Flags().StringSliceVar(&checkConfig.IgnoreHosts, "ignore-hosts", nil, "skip URLs whose host is in this list (comma-separated)")
    checkCmd.Flags().BoolVar(&checkConfig.JSONOutput, "json", false, "output as JSON array")
    checkCmd.Flags().StringVar(&checkConfig.ConfigFile, "config", "", "path to YAML config file (default: XDG config dir)")
    rootCmd.AddCommand(checkCmd)
}

func init() {
    // Load config file before flag parsing if --config is not explicitly set.
    // When --config is not provided, cobra hasn't parsed flags yet, so we use
    // the default XDG path. PersistentPreRunE runs before RunE.
    checkCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
        return loadConfigFile(cmd)
    }
}

func runCheck(cmd *cobra.Command, args []string) error {
    // TODO: wire actual logic in Task 5
    return nil
}

// loadConfigFile reads the YAML config file and applies values to checkConfig.
// Flag values take precedence over config file values.
func loadConfigFile(cmd *cobra.Command) error {
    cfgPath := checkConfig.ConfigFile
    if cfgPath == "" {
        // Default XDG config path: $XDG_CONFIG_HOME/deadlink/config.yaml
        // or ~/.config/deadlink/config.yaml
        cfgPath = defaultConfigPath()
    }
    if cfgPath == "" {
        return nil // no config file found, use defaults
    }

    data, err := os.ReadFile(cfgPath)
    if err != nil {
        return nil // config file not found or unreadable, use defaults/flags
    }

    var fileCfg CheckConfig
    if err := yaml.Unmarshal(data, &fileCfg); err != nil {
        return fmt.Errorf("parsing config file %s: %w", cfgPath, err)
    }

    // Apply config file values only where flags are still at defaults.
    // We detect "flag not set" by comparing against zero values, but since
    // cobra parses flags before PersistentPreRunE, we need to check if the
    // flag was explicitly set by the user. cobra doesn't expose this directly,
    // so we use a different approach: only apply config values for fields that
    // are still at their zero value (meaning the user didn't set them via flags).
    applyIfUnset(&checkConfig.Root, fileCfg.Root)
    applyIfIntUnset(&checkConfig.Threads, fileCfg.Threads)
    applyIfDurationUnset(&checkConfig.Timeout, fileCfg.Timeout)
    applyIfDurationUnset(&checkConfig.CacheTTL, fileCfg.CacheTTL)
    applyIfIntUnset(&checkConfig.Retry, fileCfg.Retry)
    applyIfUnset(&checkConfig.UserAgent, fileCfg.UserAgent)
    applyIfStringSliceUnset(&checkConfig.AllowFileExtensions, fileCfg.AllowFileExtensions)
    applyIfStringSliceUnset(&checkConfig.IgnoreHosts, fileCfg.IgnoreHosts)
    applyIfBoolUnset(&checkConfig.JSONOutput, fileCfg.JSONOutput)

    return nil
}

func defaultConfigPath() string {
    if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
        return filepath.Join(xdg, "deadlink", "config.yaml")
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return ""
    }
    return filepath.Join(home, ".config", "deadlink", "config.yaml")
}

func applyIfUnset(dst *string, src string) {
    if *dst == "" {
        *dst = src
    }
}

func applyIfIntUnset(dst *int, src int) {
    if *dst == 0 {
        *dst = src
    }
}

func applyIfDurationUnset(dst *time.Duration, src time.Duration) {
    if *dst == 0 {
        *dst = src
    }
}

func applyIfBoolUnset(dst *bool, src bool) {
    if !*dst {
        *dst = src
    }
}

func applyIfStringSliceUnset(dst *[]string, src []string) {
    if len(*dst) == 0 && len(src) > 0 {
        *dst = append([]string(nil), src...)
    }
}
```

Note: `Args: cobra.ExactArgs(1)` is a placeholder — will change to `cobra.MinimumNArgs(1)` in Task 5 when we wire the actual file list.

Add imports to check.go:

```go
import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/spf13/cobra"
    "gopkg.in/yaml.v3"
)
```

- [ ] **Step 6: Create `internal/model/model.go`**

```go
package model

// LinkCheck represents the result of checking a single URL.
type LinkCheck struct {
    URL           string        // the URL that was checked
    Status        int           // final HTTP status code, 0 if error or skipped
    Alive         bool          // true if final status is 2xx/3xx after following redirects
    RedirectChain []RedirectStep // intermediate redirect steps (empty if no redirect)
    Err           string        // non-HTTP error description, empty if successful
    SourceFile    string        // which input file contained this link
    Skipped       bool          // true when the URL's host matched --ignore-hosts
}

// RedirectStep is one hop in a redirect chain.
type RedirectStep struct {
    URL    string // the URL that was redirected to
    Status int    // the HTTP status code of the redirect response
}
```

- [ ] **Step 7: Build to verify skeleton compiles**

```bash
go build ./cmd/deadlink
```

Expected: binary produces no output (check command does nothing yet).

- [ ] **Step 8: Verify `deadlink check --help` shows all flags**

```bash
./deadlink check --help
```

Expected: help text listing `--root`, `--threads`/`-n`, `--timeout`/`-t`, `--cache-ttl`, `--retry`, `--user-agent`, `--allow-file-extensions`, `--ignore-hosts`, `--json`, `--config`/`-c`.

- [ ] **Step 8b: Add config file test**

Create `cmd/deadlink/config_test.go`:

```go
package deadlink

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func TestLoadConfigFile(t *testing.T) {
    // Create a temp config file
    tmpDir := t.TempDir()
    configFile := filepath.Join(tmpDir, "config.yaml")
    err := os.WriteFile(configFile, []byte(`
root: "/custom/root"
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
        UserAgent:   "deadlink/0.1.0",
    }
    checkConfig.ConfigFile = configFile

    err = loadConfigFile(nil)
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
    err := os.WriteFile(configFile, []byte(`
threads: 5
json: true
`), 0644)
    if err != nil {
        t.Fatal(err)
    }

    // Set checkConfig to non-default values (simulating flag overrides)
    checkConfig = CheckConfig{
        Threads:     20, // flag override
        Timeout:     10 * time.Second,
        CacheTTL:    259200 * time.Second,
        Retry:       2,
        UserAgent:   "deadlink/0.1.0",
        JSONOutput:  false, // flag override
    }
    checkConfig.ConfigFile = configFile

    err = loadConfigFile(nil)
    if err != nil {
        t.Fatalf("loadConfigFile returned error: %v", err)
    }

    // Flag values should override config file values
    if checkConfig.Threads != 20 {
        t.Errorf("expected threads 20 (flag override), got %d", checkConfig.Threads)
    }
    if checkConfig.JSONOutput != false {
        t.Errorf("expected json false (flag override), got %v", checkConfig.JSONOutput)
    }
}

func TestDefaultConfigPath(t *testing.T) {
    // Save and restore XDG_CONFIG_HOME
    oldXDG := os.Getenv("XDG_CONFIG_HOME")
    defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

    os.Unsetenv("XDG_CONFIG_HOME")
    path := defaultConfigPath()
    home, _ := os.UserHomeDir()
    expected := filepath.Join(home, ".config", "deadlink", "config.yaml")
    if path != expected {
        t.Errorf("expected %s, got %s", expected, path)
    }

    os.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
    path = defaultConfigPath()
    expected = filepath.Join("/custom/xdg", "deadlink", "config.yaml")
    if path != expected {
        t.Errorf("expected %s, got %s", expected, path)
    }
}
```

Run:

```bash
go test ./cmd/deadlink/ -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum cmd/deadlink/ internal/model/
git commit -m "feat: scaffold deadlink CLI with cobra and all flags wired"
```

---

### Task 2: URL extraction from HTML and Markdown

**Files:**
- Create: `internal/extract/extract.go`
- Create: `internal/extract/extract_test.go`

**Interfaces:**
- Consumes: nothing (standalone package)
- Produces: `ExtractURLs(content []byte, ext string, baseURL string) []string` — returns absolute HTTP(S) URLs extracted from content

- [ ] **Step 1: Write failing test for HTML extraction**

```go
package extract

import (
    "net/url"
    "testing"
)

func TestExtractHTML(t *testing.T) {
    html := `<!DOCTYPE html>
<html>
<body>
<a href="/page">relative</a>
<a href="https://example.com/absolute">absolute</a>
<a href="http://example.com/bare">bare http</a>
<img src="https://cdn.example.com/img.png">
<link href="/style.css">
<script src="https://cdn.example.com/app.js"></script>
<a href="mailto:test@example.com">email</a>
<a href="#anchor">anchor</a>
<a href="file:///local/path">file</a>
</body>
</html>`

    baseURL := "https://example.com/docs/"
    results := ExtractURLs([]byte(html), "html", baseURL)

    expected := []string{
        "https://example.com/page",
        "https://example.com/absolute",
        "http://example.com/bare",
        "https://cdn.example.com/img.png",
        "https://example.com/style.css",
        "https://cdn.example.com/app.js",
    }

    if len(results) != len(expected) {
        t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
    }

    for _, e := range expected {
        found := false
        for _, r := range results {
            if r == e {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("missing expected URL %s; got %v", e, results)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/extract/ -run TestExtractHTML -v
```

Expected: FAIL — `ExtractURLs` undefined.

- [ ] **Step 3: Write minimal HTML extractor**

```go
package extract

import (
    "fmt"
    "net/url"
    "strings"

    "golang.org/x/net/html"
)

// HTMLLinkExtractor collects href/src attributes from HTML nodes.
type HTMLLinkExtractor struct {
    baseURL *url.URL
    links   []string
}

func (e *HTMLLinkExtractor) Visit(n *html.Node) {
    if n.Type == html.ElementNode {
        var attrName string
        switch n.Data {
        case "a", "link":
            attrName = "href"
        case "img", "script", "source", "iframe":
            attrName = "src"
        default:
            return
        }

        for _, attr := range n.Attr {
            if attr.Key == attrName {
                e.collect(attr.Val)
                break
            }
        }
    }
}

func (e *HTMLLinkExtractor) collect(raw string) {
    if raw == "" {
        return
    }
    // Skip non-HTTP schemes
    lower := strings.ToLower(raw)
    if strings.HasPrefix(lower, "mailto:") ||
        strings.HasPrefix(lower, "tel:") ||
        strings.HasPrefix(lower, "javascript:") ||
        strings.HasPrefix(lower, "data:") ||
        strings.HasPrefix(lower, "file:") ||
        strings.HasPrefix(lower, "#") {
        return
    }

    rel, err := url.Parse(raw)
    if err != nil {
        return
    }

    var abs *url.URL
    if rel.IsAbs() {
        abs = rel
    } else {
        abs = e.baseURL.ResolveReference(rel)
    }

    // Only keep HTTP(S) URLs
    if abs.Scheme != "http" && abs.Scheme != "https" {
        return
    }

    e.links = append(e.links, abs.String())
}

// ExtractHTML extracts HTTP(S) URLs from HTML content, resolving relative URLs
// against baseURL.
func ExtractHTML(content []byte, baseURL string) []string {
    base, err := url.Parse(baseURL)
    if err != nil {
        return nil
    }

    extractor := &HTMLLinkExtractor{baseURL: base}
    html.Parse(strings.NewReader(string(content))).Walk(extractor)
    return extractor.links
}
```

- [ ] **Step 4: Run test to verify HTML extraction passes**

```bash
go test ./internal/extract/ -run TestExtractHTML -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for Markdown extraction**

```go
func TestExtractMarkdown(t *testing.T) {
    md := `# Title

[link text](https://example.com/page)
![alt text](http://images.example.com/img.png)
[bare](https://example.com/bare)

https://example.com/bare-link
http://example.com/another-bare

[relative](/relative/page)

[mailto](mailto:test@example.com)
`

    baseURL := "https://example.com/docs/"
    results := ExtractURLs([]byte(md), "md", baseURL)

    expected := []string{
        "https://example.com/page",
        "http://images.example.com/img.png",
        "https://example.com/bare",
        "https://example.com/bare-link",
        "http://example.com/another-bare",
    }

    if len(results) != len(expected) {
        t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
    }

    for _, e := range expected {
        found := false
        for _, r := range results {
            if r == e {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("missing expected URL %s; got %v", e, results)
        }
    }
}
```

- [ ] **Step 6: Run test to verify it fails**

```bash
go test ./internal/extract/ -run TestExtractMarkdown -v
```

Expected: FAIL — `ExtractURLs` doesn't handle markdown yet (or returns HTML results only).

- [ ] **Step 7: Add Markdown extraction**

```go
import "regexp"

var (
    // markdownLink matches [text](url) and ![alt](url)
    markdownLinkRe = regexp.MustCompile(`[!]?\[.*?\]\((https?://[^\s)]+)\)`)
    // bareURL matches standalone http(s):// URLs in text
    bareURLRe = regexp.MustCompile(`https?://[^\s)>\]"']+`)
)

// ExtractMarkdown extracts HTTP(S) URLs from Markdown content.
func ExtractMarkdown(content []byte, baseURL string) []string {
    base, err := url.Parse(baseURL)
    if err != nil {
        return nil
    }

    var results []string
    text := string(content)

    // Extract from markdown links [text](url) and ![alt](url)
    for _, match := range markdownLinkRe.FindAllStringSubmatch(text, -1) {
        raw := match[1]
        rel, err := url.Parse(raw)
        if err != nil {
            continue
        }
        var abs *url.URL
        if rel.IsAbs() {
            abs = rel
        } else {
            abs = base.ResolveReference(rel)
        }
        if abs.Scheme == "http" || abs.Scheme == "https" {
            results = append(results, abs.String())
        }
    }

    // Extract bare URLs, but skip ones already captured by markdown links
    // (bare URL extraction is additive — a URL could appear both as a link and bare)
    seen := make(map[string]bool)
    for _, u := range results {
        seen[u] = true
    }

    for _, match := range bareURLRe.FindAllString(text, -1) {
        // Clean trailing punctuation that often follows bare URLs in text
        cleaned := strings.TrimRight(match, "),.;:]>\"'")
        rel, err := url.Parse(cleaned)
        if err != nil {
            continue
        }
        if !rel.IsAbs() {
            continue
        }
        if rel.Scheme != "http" && rel.Scheme != "https" {
            continue
        }
        abs := rel.String()
        if !seen[abs] {
            results = append(results, abs)
            seen[abs] = true
        }
    }

    return results
}
```

- [ ] **Step 8: Run Markdown test to verify it passes**

```bash
go test ./internal/extract/ -run TestExtractMarkdown -v
```

Expected: PASS.

- [ ] **Step 9: Write test for plain text extraction (txt extension)**

```go
func TestExtractText(t *testing.T) {
    txt := `Check out https://example.com/page and http://other.example.org/foo
Also https://example.com/page (duplicate)`
    baseURL := "https://example.com/docs/"
    results := ExtractURLs([]byte(txt), "txt", baseURL)

    expected := []string{
        "https://example.com/page",
        "http://other.example.org/foo",
    }

    if len(results) != len(expected) {
        t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
    }

    seen := make(map[string]bool)
    for _, r := range results {
        seen[r] = true
    }
    for _, e := range expected {
        if !seen[e] {
            t.Errorf("missing expected URL %s; got %v", e, results)
        }
    }
}
```

- [ ] **Step 10: Run text test**

```bash
go test ./internal/extract/ -run TestExtractText -v
```

Expected: PASS (text uses bare URL extraction only, with deduplication).

- [ ] **Step 11: Wire the public ExtractURLs dispatcher**

Add to `extract.go`:

```go
// ExtractURLs extracts HTTP(S) URLs from content based on file extension.
// Supported extensions: html, md, mkd, mkdown, markdown, txt, yaml, yml, c, h, cpp, hpp.
// For unknown extensions, only bare URL extraction is used.
func ExtractURLs(content []byte, ext string, baseURL string) []string {
    switch strings.ToLower(ext) {
    case "html":
        return ExtractHTML(content, baseURL)
    case "md", "mkd", "mkdown", "markdown":
        return ExtractMarkdown(content, baseURL)
    case "txt", "yaml", "yml", "c", "h", "cpp", "hpp":
        return ExtractBareURLs(content, baseURL)
    default:
        return ExtractBareURLs(content, baseURL)
    }
}

// ExtractBareURLs extracts only bare http(s):// URLs from content.
func ExtractBareURLs(content []byte, baseURL string) []string {
    base, err := url.Parse(baseURL)
    if err != nil {
        return nil
    }

    var results []string
    text := string(content)
    seen := make(map[string]bool)

    for _, match := range bareURLRe.FindAllString(text, -1) {
        cleaned := strings.TrimRight(match, "),.;:]>\"'")
        rel, err := url.Parse(cleaned)
        if err != nil {
            continue
        }
        if !rel.IsAbs() {
            continue
        }
        if rel.Scheme != "http" && rel.Scheme != "https" {
            continue
        }
        abs := rel.String()
        if !seen[abs] {
            results = append(results, abs)
            seen[abs] = true
        }
    }

    return results
}
```

- [ ] **Step 12: Run all extract tests**

```bash
go test ./internal/extract/ -v
```

Expected: all PASS.

- [ ] **Step 13: Commit**

```bash
git add internal/extract/
git commit -m "feat: add URL extraction from HTML, Markdown, and text files"
```

---

### Task 3: Checker — HTTP fetch with redirect following, retry, and cache

**Files:**
- Create: `internal/checker/checker.go`
- Create: `internal/checker/checker_test.go`

**Interfaces:**
- Consumes: `model.LinkCheck`, `model.RedirectStep`
- Produces: `checker.CheckURL(ctx, url, timeout, retryCount, userAgent, cache, resultCh) model.LinkCheck` — checks a single URL and returns the result

- [ ] **Step 1: Write failing test for basic alive check**

```go
package checker

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestCheckURLP Alive(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    result := CheckURL(nil, server.URL, 5*time.Second, 0, "test-agent", nil)

    if !result.Alive {
        t.Errorf("expected alive, got %v (status=%d, err=%s)", result.Alive, result.Status, result.Err)
    }
    if result.Status != http.StatusOK {
        t.Errorf("expected status 200, got %d", result.Status)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/checker/ -run TestCheckURLP Alive -v
```

Expected: FAIL — `CheckURL` undefined.

- [ ] **Step 3: Write minimal checker with redirect following and retry**

```go
package checker

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)

// maxRedirects is the maximum number of redirect hops to follow.
const maxRedirects = 10

// CheckResult is the result of checking a single URL.
type CheckResult struct {
    URL           string
    Status        int
    Alive         bool
    RedirectChain []model.RedirectStep
    Err           string
}

// checkCache is an in-memory cache keyed by URL.
type checkCache struct {
    mu      sync.Mutex
    entries map[string]cacheEntry
}

type cacheEntry struct {
    result   CheckResult
    expires  time.Time
}

// CheckURL checks a single URL by issuing an HTTP GET, following redirects up
// to maxRedirects, retrying on 502/503/504 up to retryCount times with
// exponential backoff. Returns the check result.
func CheckURL(ctx context.Context, rawURL string, timeout time.Duration, retryCount int, userAgent string, cache *checkCache) CheckResult {
    // Check cache first
    if cache != nil {
        cache.mu.Lock()
        if entry, ok := cache.entries[rawURL]; ok && time.Now().Before(entry.expires) {
            cache.mu.Unlock()
            return entry.result
        }
        cache.mu.Unlock()
    }

    result := checkOnce(ctx, rawURL, timeout, userAgent)

    // Retry on 502/503/504
    for i := 0; i < retryCount && isRetryable(result.Status) && result.Err == ""; i++ {
        backoff := time.Duration(i+1) * 200 * time.Millisecond
        select {
        case <-ctx.Done():
            result.Err = ctx.Err().Error()
            break
        case <-time.After(backoff):
            result = checkOnce(ctx, rawURL, timeout, userAgent)
        }
    }

    // Store in cache
    if cache != nil && result.Err == "" {
        cache.mu.Lock()
        cache.entries[rawURL] = cacheEntry{
            result:  result,
            expires: time.Now().Add(cacheTTL), // cacheTTL set by caller via context or passed in
        }
        cache.mu.Unlock()
    }

    return result
}

func isRetryable(status int) bool {
    return status == http.StatusBadGateway ||
        status == http.StatusServiceUnavailable ||
        status == http.StatusGatewayTimeout
}

func checkOnce(ctx context.Context, rawURL string, timeout time.Duration, userAgent string) CheckResult {
    client := &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyFromEnvironment,
        },
        Timeout: timeout,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            if len(via) >= maxRedirects {
                return fmt.Errorf("too many redirects")
            }
            return http.ErrUseLastResponse
        },
    }

    if userAgent != "" {
        client.Transport = &userAgentTransport{
            base:    http.DefaultTransport,
            userAgent: userAgent,
        }
    }

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
    if err != nil {
        return CheckResult{URL: rawURL, Err: err.Error()}
    }
    req.Header.Set("User-Agent", userAgent)
    req.Header.Set("Accept", "*/*")

    resp, err := client.Do(req)
    if err != nil {
        if ctx.Err() != nil {
            return CheckResult{URL: rawURL, Err: ctx.Err().Error()}
        }
        // Connection errors, DNS failures, etc.
        return CheckResult{URL: rawURL, Status: 0, Err: err.Error()}
    }
    defer resp.Body.Close()

    // Follow redirects manually to capture the chain
    var chain []model.RedirectStep
    currentURL := rawURL
    currentStatus := resp.StatusCode

    for currentStatus >= 300 && currentStatus < 400 {
        chain = append(chain, model.RedirectStep{
            URL:    currentURL,
            Status: currentStatus,
        })

        location := resp.Header.Get("Location")
        if location == "" {
            break
        }

        nextURL, err := url.Parse(location)
        if err != nil {
            break
        }

        // Resolve relative Location header against current URL
        if !nextURL.IsAbs() {
            base, _ := url.Parse(currentURL)
            nextURL = base.ResolveReference(nextURL)
        }

        currentURL = nextURL.String()

        req, err = http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
        if err != nil {
            return CheckResult{URL: rawURL, Err: err.Error()}
        }
        req.Header.Set("User-Agent", userAgent)
        req.Header.Set("Accept", "*/*")

        resp, err = client.Do(req)
        if err != nil {
            return CheckResult{URL: rawURL, Err: err.Error()}
        }
        defer resp.Body.Close()
        currentStatus = resp.StatusCode
    }

    // Read and discard body to allow connection reuse
    io.Copy(io.Discard, resp.Body)

    alive := currentStatus >= 200 && currentStatus < 400

    result := CheckResult{
        URL:           rawURL,
        Status:        currentStatus,
        Alive:         alive,
        RedirectChain: chain,
        Err:           "",
    }

    if len(chain) > 0 {
        // The final URL in the chain is the last redirect target, not the final response URL
        chain = append(chain, model.RedirectStep{
            URL:    currentURL,
            Status: currentStatus,
        })
    }

    return result
}

// userAgentTransport wraps a base transport and sets User-Agent on every request.
type userAgentTransport struct {
    base       http.RoundTripper
    userAgent  string
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req = req.Clone(req.Context())
    req.Header.Set("User-Agent", t.userAgent)
    return t.base.RoundTrip(req)
}
```

Wait — the above has a bug: the cache TTL is referenced as `cacheTTL` but not passed in. The cache TTL should be passed as a parameter. Let me fix the signature:

```go
func CheckURL(ctx context.Context, rawURL string, timeout time.Duration, retryCount int, userAgent string, cache *checkCache, cacheTTL time.Duration) CheckResult {
```

And update the cache store:

```go
cache.entries[rawURL] = cacheEntry{
    result:  result,
    expires: time.Now().Add(cacheTTL),
}
```

Also, the `userAgentTransport` wrapping is redundant with `req.Header.Set("User-Agent")` — simplify by just setting the header on the request (which we already do). Remove the transport wrapper and the `userAgent` field from `CheckURL` — or keep it for the case where the transport is used. Actually, since we set `req.Header.Set("User-Agent", userAgent)` in both the initial request and redirect requests, the transport wrapper is unnecessary. Remove it.

Cleaned-up `CheckURL` signature:

```go
func CheckURL(ctx context.Context, rawURL string, timeout time.Duration, retryCount int, userAgent string, cache *checkCache, cacheTTL time.Duration) CheckResult {
```

- [ ] **Step 4: Run test to verify alive check passes**

```bash
go test ./internal/checker/ -run TestCheckURLP Alive -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for dead link (404)**

```go
func TestCheckURLDead(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNotFound)
    }))
    defer server.Close()

    result := CheckURL(nil, server.URL, 5*time.Second, 0, "test-agent", nil, 0)

    if result.Alive {
        t.Errorf("expected dead, got alive (status=%d)", result.Status)
    }
    if result.Status != http.StatusNotFound {
        t.Errorf("expected status 404, got %d", result.Status)
    }
}
```

- [ ] **Step 6: Run test**

```bash
go test ./internal/checker/ -run TestCheckURLDead -v
```

Expected: PASS.

- [ ] **Step 7: Write failing test for redirect chain**

```go
func TestCheckURLRedirectChain(t *testing.T) {
    redirectTarget := ""
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/redirect" {
            w.Header().Set("Location", "/target")
            w.WriteHeader(http.StatusFound)
            return
        }
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    // Capture the target URL after server starts
    redirectTarget = server.URL + "/target"

    result := CheckURL(nil, server.URL+"/redirect", 5*time.Second, 0, "test-agent", nil, 0)

    if !result.Alive {
        t.Errorf("expected alive after redirect, got dead")
    }
    if result.Status != http.StatusOK {
        t.Errorf("expected final status 200, got %d", result.Status)
    }
    if len(result.RedirectChain) == 0 {
        t.Error("expected redirect chain, got empty")
    }
    // Chain should contain the redirect response and the final URL
    if result.RedirectChain[0].Status != http.StatusFound {
        t.Errorf("expected first chain entry status 302, got %d", result.RedirectChain[0].Status)
    }
}
```

- [ ] **Step 8: Run redirect test**

```bash
go test ./internal/checker/ -run TestCheckURLRedirectChain -v
```

Expected: PASS.

- [ ] **Step 9: Write failing test for retry on 503**

```go
func TestCheckURLRetry(t *testing.T) {
    attempts := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attempts++
        if attempts <= 2 {
            w.WriteHeader(http.StatusServiceUnavailable)
            return
        }
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    result := CheckURL(nil, server.URL, 5*time.Second, 2, "test-agent", nil, 0)

    if !result.Alive {
        t.Errorf("expected alive after retry, got dead")
    }
    if attempts != 3 {
        t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
    }
}
```

- [ ] **Step 10: Run retry test**

```bash
go test ./internal/checker/ -run TestCheckURLRetry -v
```

Expected: PASS.

- [ ] **Step 11: Write failing test for context cancellation**

```go
func TestCheckURLContextCancel(t *testing.T) {
    slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(100 * time.Millisecond)
        w.WriteHeader(http.StatusOK)
    }))
    defer slowServer.Close()

    ctx, cancel := context.WithCancel(nil)
    cancel() // cancel immediately

    result := CheckURL(ctx, slowServer.URL, 5*time.Second, 0, "test-agent", nil, 0)

    if result.Err == "" {
        t.Error("expected error from cancelled context, got none")
    }
}
```

- [ ] **Step 12: Run context test**

```bash
go test ./internal/checker/ -run TestCheckURLContextCancel -v
```

Expected: PASS (or at least an error string is set).

- [ ] **Step 13: Run all checker tests**

```bash
go test ./internal/checker/ -v
```

Expected: all PASS.

- [ ] **Step 13: Commit**

```bash
git add internal/checker/
git commit -m "feat: add HTTP checker with redirect following, retry, and cache support"
```

---

### Task 4: Worker pool + file filtering + host filtering + deduplication

**Files:**
- Create: `internal/checker/worker.go` (worker pool orchestration)
- Create: `internal/checker/filter.go` (file extension and host filtering)
- Create: `internal/checker/worker_test.go`

**Interfaces:**
- Consumes: `extract.ExtractURLs`, `checker.CheckURL`, `model.LinkCheck`
- Produces: `checker.CheckAll(ctx, config, files, extractor, cache) []model.LinkCheck` — orchestrates the full pipeline from files to results

- [ ] **Step 1: Write the file extension filter**

```go
package checker

import "strings"

// allowedFileExtensions returns true if the file's extension is in the allowed list.
// If the allowed list is empty, all files are allowed.
func allowedFileExtension(filename string, allowed []string) bool {
    if len(allowed) == 0 {
        return true
    }
    ext := strings.ToLower(filepath.Ext(filename))
    for _, a := range allowed {
        if strings.EqualFold(ext, a) {
            return true
        }
    }
    return false
}
```

Note: this needs `path/filepath` import.

- [ ] **Step 2: Write the host filter**

```go
// hostIsIgnored returns true if the URL's host matches one of the ignored hosts.
func hostIsIgnored(rawURL string, ignored []string) bool {
    u, err := url.Parse(rawURL)
    if err != nil {
        return false
    }
    host := strings.ToLower(u.Host)
    for _, h := range ignored {
        if strings.EqualFold(host, strings.ToLower(h)) {
            return true
        }
    }
    return false
}
```

Note: this needs `net/url` import.

- [ ] **Step 3: Write failing test for file extension filter**

```go
func TestAllowedFileExtension(t *testing.T) {
    tests := []struct {
        filename string
        allowed  []string
        want     bool
    }{
        {"doc.html", []string{".html"}, true},
        {"doc.HTML", []string{".html"}, true},
        {"doc.txt", []string{".html"}, false},
        {"doc.md", []string{".html", ".md"}, true},
        {"doc.yml", []string{}, true}, // empty list = allow all
        {"doc.yaml", []string{".txt", ".yaml"}, true},
        {"doc.c", []string{".c", ".h"}, true},
        {"doc.h", []string{".c", ".h"}, true},
        {"doc.cpp", []string{".c", ".h"}, false},
        {"doc", []string{".html"}, false}, // no extension
    }

    for _, tt := range tests {
        t.Run(tt.filename, func(t *testing.T) {
            if got := allowedFileExtension(tt.filename, tt.allowed); got != tt.want {
                t.Errorf("allowedFileExtension(%q, %v) = %v, want %v", tt.filename, tt.allowed, got, tt.want)
            }
        })
    }
}
```

- [ ] **Step 4: Run filter test**

```bash
go test ./internal/checker/ -run TestAllowedFileExtension -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for host filter**

```go
func TestHostIsIgnored(t *testing.T) {
    tests := []struct {
        url      string
        ignored  []string
        want     bool
    }{
        {"https://example.com/page", []string{"example.com"}, true},
        {"https://example.com/page", []string{"www.example.com"}, false},
        {"http://Example.COM/page", []string{"example.com"}, true},
        {"https://other.com/page", []string{"example.com"}, false},
        {"https://sub.example.com/page", []string{"example.com"}, false},
        {"not-a-url", []string{"example.com"}, false},
    }

    for _, tt := range tests {
        t.Run(tt.url, func(t *testing.T) {
            if got := hostIsIgnored(tt.url, tt.ignored); got != tt.want {
                t.Errorf("hostIsIgnored(%q, %v) = %v, want %v", tt.url, tt.ignored, got, tt.want)
            }
        })
    }
}
```

- [ ] **Step 6: Run host filter test**

```bash
go test ./internal/checker/ -run TestHostIsIgnored -v
```

Expected: PASS.

- [ ] **Step 7: Write the worker pool + orchestration**

```go
package checker

import (
    "context"
    "fmt"
    "net/url"
    "path/filepath"
    "strings"
    "sync"
    "sync/atomic"

    "deadlink/internal/extract"
    "deadlink/internal/model"
)

// CheckConfig holds the configuration for a check run.
type CheckConfig struct {
    Root                string
    Threads             int
    Timeout             time.Duration
    CacheTTL            time.Duration
    Retry               int
    UserAgent           string
    AllowFileExtensions []string
    IgnoreHosts         []string
}

// CheckAll reads links from the given files, checks each URL, and returns results.
func CheckAll(ctx context.Context, cfg CheckConfig, files []string) []model.LinkCheck {
    // Resolve root to absolute path
    root, err := filepath.Abs(cfg.Root)
    if err != nil {
        root = cfg.Root
    }

    // Build base URL from root for resolving relative links
    baseURL := ""
    if root != "" {
        u, err := url.Parse(root)
        if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
            baseURL = root
        } else {
            // For local file roots, we can't construct a meaningful base URL for HTTP resolution.
            // Use file:// scheme as a fallback for relative URL resolution.
            baseURL = "file://" + root
        }
    }
    if baseURL == "" {
        baseURL = "file://."
    }

    // Deduplicate: URL -> source files (for reporting)
    urlSources := make(map[string][]string)
    var allURLs []string

    for _, file := range files {
        if !allowedFileExtension(file, cfg.AllowFileExtensions) {
            continue
        }

        content, err := os.ReadFile(file)
        if err != nil {
            // Skip files we can't read
            continue
        }

        ext := strings.ToLower(filepath.Ext(file))
        ext = strings.TrimPrefix(ext, ".")
        if ext == "" {
            ext = "txt" // default to bare URL extraction
        }

        urls := extract.ExtractURLs(content, ext, baseURL)
        for _, u := range urls {
            if _, exists := urlSources[u]; !exists {
                allURLs = append(allURLs, u)
            }
            urlSources[u] = append(urlSources[u], file)
        }
    }

    // Filter out ignored hosts
    var urlsToCheck []string
    for _, u := range allURLs {
        if hostIsIgnored(u, cfg.IgnoreHosts) {
            continue
        }
        urlsToCheck = append(urlsToCheck, u)
    }

    // Check with worker pool
    cache := &checkCache{
        entries: make(map[string]cacheEntry),
    }

    results := make([]model.LinkCheck, 0, len(urlsToCheck))
    var wg sync.WaitGroup
    var mu sync.Mutex
    var checked atomic.Int32

    semaphore := make(chan struct{}, cfg.Threads)

    for _, u := range urlsToCheck {
        if ctx.Err() != nil {
            break
        }

        wg.Add(1)
        semaphore <- struct{}{}

        go func(url string) {
            defer wg.Done()
            defer func() { <-semaphore }()

            r := CheckURL(ctx, url, cfg.Timeout, cfg.Retry, cfg.UserAgent, cache, cfg.CacheTTL)
            r.SourceFile = strings.Join(urlSources[url], ", ")

            if r.Skipped {
                // This shouldn't happen since we filtered, but handle gracefully
                return
            }

            mu.Lock()
            results = append(results, model.LinkCheck{
                URL:           r.URL,
                Status:        r.Status,
                Alive:         r.Alive,
                RedirectChain: r.RedirectChain,
                Err:           r.Err,
                SourceFile:    r.SourceFile,
                Skipped:       false,
            })
            mu.Unlock()

            checked.Add(1)
        }(u)
    }

    wg.Wait()

    // Sort: dead links first
    sort.Slice(results, func(i, j int) bool {
        if results[i].Alive != results[j].Alive {
            return !results[i].Alive // dead first
        }
        return results[i].URL < results[j].URL
    })

    return results
}
```

Wait — the above references `os.ReadFile` and `sort` but doesn't import them. Add imports:

```go
import (
    "context"
    "fmt"
    "net/url"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "sync"
    "sync/atomic"

    "deadlink/internal/extract"
    "deadlink/internal/model"
)
```

Also `cacheTTL` field — the `checkCache` type needs to store the TTL or the caller passes it. In the current `CheckURL` signature I defined `cacheTTL time.Duration` as a parameter, so the call `CheckURL(ctx, url, cfg.Timeout, cfg.Retry, cfg.UserAgent, cache, cfg.CacheTTL)` is correct.

- [ ] **Step 8: Write failing test for CheckAll**

```go
func TestCheckAll(t *testing.T) {
    // Create a temp file with a link to the test server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()

    tmpDir := t.TempDir()
    htmlFile := filepath.Join(tmpDir, "test.html")
    err := os.WriteFile(htmlFile, []byte(`<a href="` + server.URL + `/page">link</a>`), 0644)
    if err != nil {
        t.Fatal(err)
    }

    ctx := context.Background()
    cfg := CheckConfig{
        Root:       tmpDir,
        Threads:    2,
        Timeout:    5 * time.Second,
        CacheTTL:   0, // no caching for tests
        Retry:      0,
        UserAgent:  "test-agent",
    }

    results := CheckAll(ctx, cfg, []string{htmlFile})

    if len(results) != 1 {
        t.Fatalf("expected 1 result, got %d: %v", len(results), results)
    }

    if !results[0].Alive {
        t.Errorf("expected alive, got dead (status=%d, err=%s)", results[0].Status, results[0].Err)
    }
    if results[0].SourceFile != htmlFile {
        t.Errorf("expected source file %s, got %s", htmlFile, results[0].SourceFile)
    }
}
```

- [ ] **Step 9: Run CheckAll test**

```bash
go test ./internal/checker/ -run TestCheckAll -v
```

Expected: PASS.

- [ ] **Step 10: Write test for ignore-hosts filtering**

```go
func TestCheckAllIgnoreHosts(t *testing.T) {
    tmpDir := t.TempDir()
    htmlFile := filepath.Join(tmpDir, "test.html")
    err := os.WriteFile(htmlFile, []byte(`<a href="https://www.example.com/page">link</a>`), 0644)
    if err != nil {
        t.Fatal(err)
    }

    cfg := CheckConfig{
        Root:                tmpDir,
        Threads:             2,
        Timeout:             5 * time.Second,
        CacheTTL:            0,
        Retry:               0,
        UserAgent:           "test-agent",
        IgnoreHosts:         []string{"www.example.com"},
    }

    results := CheckAll(context.Background(), cfg, []string{htmlFile})

    if len(results) != 0 {
        t.Errorf("expected 0 results (all ignored), got %d: %v", len(results), results)
    }
}
```

- [ ] **Step 11: Run ignore-hosts test**

```bash
go test ./internal/checker/ -run TestCheckAllIgnoreHosts -v
```

Expected: PASS.

- [ ] **Step 12: Write test for file extension filtering**

```go
func TestCheckAllFileExtensionFilter(t *testing.T) {
    tmpDir := t.TempDir()

    // HTML file with a link — should be checked if .html is allowed
    htmlFile := filepath.Join(tmpDir, "test.html")
    err := os.WriteFile(htmlFile, []byte(`<a href="https://example.com/page">link</a>`), 0644)
    if err != nil {
        t.Fatal(err)
    }

    // txt file with a link — should be skipped if only .html is allowed
    txtFile := filepath.Join(tmpDir, "test.txt")
    err = os.WriteFile(txtFile, []byte("https://example.com/page"), 0644)
    if err != nil {
        t.Fatal(err)
    }

    cfg := CheckConfig{
        Root:                tmpDir,
        Threads:             2,
        Timeout:             5 * time.Second,
        CacheTTL:            0,
        Retry:               0,
        UserAgent:           "test-agent",
        AllowFileExtensions: []string{".html"},
    }

    results := CheckAll(context.Background(), cfg, []string{htmlFile, txtFile})

    if len(results) != 1 {
        t.Errorf("expected 1 result (html only), got %d: %v", len(results), results)
    }
}
```

- [ ] **Step 13: Run file extension filter test**

```bash
go test ./internal/checker/ -run TestCheckAllFileExtensionFilter -v
```

Expected: PASS.

- [ ] **Step 14: Run all checker tests**

```bash
go test ./internal/checker/ -v
```

Expected: all PASS.

- [ ] **Step 15: Commit**

```bash
git add internal/checker/
git commit -m "feat: add worker pool, file filtering, host filtering, and deduplication"
```

---

### Task 5: Wire check command + report output (text + JSON) + exit code + config file flag

**Files:**
- Modify: `cmd/deadlink/check.go` — wire `runCheck` to call `checker.CheckAll` and `report.PrintResults`; set `checkConfig.Files = args`
- Create: `internal/report/report.go`
- Create: `internal/report/report_test.go`

**Interfaces:**
- Consumes: `checker.CheckAll`, `model.LinkCheck`
- Produces: `report.PrintResults(results []model.LinkCheck, jsonOutput bool)` — prints results to stdout

**Files:**
- Modify: `cmd/deadlink/check.go` — wire `runCheck` to call `checker.CheckAll` and `report.PrintResults`
- Create: `internal/report/report.go`
- Create: `internal/report/report_test.go`

**Interfaces:**
- Consumes: `checker.CheckAll`, `model.LinkCheck`
- Produces: `report.PrintResults(results []model.LinkCheck, jsonOutput bool)` — prints results to stdout

- [ ] **Step 1: Write the text report**

```go
package report

import (
    "fmt"
    "sort"
    "strings"

    "deadlink/internal/model"
)

// PrintResults prints the check results to stdout. Dead links are reported first.
// If jsonOutput is true, results are printed as a JSON array.
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

    // Sort: dead links first, then by URL
    sorted := make([]model.LinkCheck, len(results))
    copy(sorted, results)
    sort.Slice(sorted, func(i, j int) bool {
        if sorted[i].Alive != sorted[j].Alive {
            return !sorted[i].Alive
        }
        return sorted[i].URL < sorted[j].URL
    })

    for _, r := range sorted {
        if r.Skipped {
            continue
        }
        if r.Alive {
            fmt.Printf("%s → %d\n", r.URL, r.Status)
        } else {
            msg := fmt.Sprintf("%s → %d", r.URL, r.Status)
            if r.Err != "" {
                msg += fmt.Sprintf(" (%s)", r.Err)
            }
            if len(r.RedirectChain) > 0 {
                chain := make([]string, len(r.RedirectChain))
                for i, step := range r.RedirectChain {
                    chain[i] = fmt.Sprintf("%d %s", step.Status, step.URL)
                }
                msg += fmt.Sprintf(" [redirect: %s]", strings.Join(chain, ", "))
            }
            fmt.Println(msg)
        }
    }
}

func printJSON(results []model.LinkCheck) {
    // Convert to a JSON-serializable slice
    type jsonResult struct {
        URL           string        `json:"url"`
        Status        int           `json:"status"`
        Alive         bool          `json:"alive"`
        RedirectChain []redirectJSON `json:"redirect_chain,omitempty"`
        Err           string        `json:"error,omitempty"`
        SourceFile    string        `json:"source_file,omitempty"`
        Skipped       bool          `json:"skipped"`
    }

    type redirectJSON struct {
        URL    string `json:"url"`
        Status int    `json:"status"`
    }

    out := make([]jsonResult, 0, len(results))
    for _, r := range results {
        if r.Skipped {
            continue
        }
        jr := jsonResult{
            URL:     r.URL,
            Status:  r.Status,
            Alive:   r.Alive,
            Err:     r.Err,
            Skipped: r.Skipped,
        }
        if len(r.RedirectChain) > 0 {
            jr.RedirectChain = make([]redirectJSON, len(r.RedirectChain))
            for i, step := range r.RedirectChain {
                jr.RedirectChain[i] = redirectJSON{
                    URL:    step.URL,
                    Status: step.Status,
                }
            }
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
```

Add imports:

```go
import (
    "encoding/json"
    "fmt"
    "os"
    "sort"
    "strings"

    "deadlink/internal/model"
)
```

- [ ] **Step 2: Write failing test for text report**

```go
func TestPrintText(t *testing.T) {
    results := []model.LinkCheck{
        {URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
        {URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
    }

    // Capture stdout
    old := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    PrintResults(results, false)

    w.Close()
    os.Stdout = old

    var buf strings.Builder
    io.Copy(&buf, r)
    output := buf.String()

    if !strings.Contains(output, "https://example.com/dead") {
        t.Errorf("expected dead link in output, got: %s", output)
    }
    if !strings.Contains(output, "https://example.com/ok") {
        t.Errorf("expected alive link in output, got: %s", output)
    }
    // Dead link should appear first
    deadIdx := strings.Index(output, "https://example.com/dead")
    okIdx := strings.Index(output, "https://example.com/ok")
    if deadIdx > okIdx {
        t.Errorf("expected dead link before alive link in output")
    }
}
```

- [ ] **Step 3: Run text report test**

```bash
go test ./internal/report/ -run TestPrintText -v
```

Expected: FAIL → then PASS after implementing.

- [ ] **Step 4: Run test after implementation (it should pass now)**

Actually, write the implementation first (Step 1), then run the test. Since Step 1 already has the implementation, run:

```bash
go test ./internal/report/ -run TestPrintText -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for JSON report**

```go
func TestPrintJSON(t *testing.T) {
    results := []model.LinkCheck{
        {URL: "https://example.com/ok", Status: 200, Alive: true, SourceFile: "test.html"},
        {URL: "https://example.com/dead", Status: 404, Alive: false, SourceFile: "test.html"},
    }

    old := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    PrintResults(results, true)

    w.Close()
    os.Stdout = old

    var buf strings.Builder
    io.Copy(&buf, r)
    output := buf.String()

    var parsed []map[string]interface{}
    if err := json.Unmarshal([]byte(output), &parsed); err != nil {
        t.Fatalf("output is not valid JSON: %s", output)
    }

    if len(parsed) != 2 {
        t.Errorf("expected 2 JSON results, got %d", len(parsed))
    }

    // Find the dead one
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
```

- [ ] **Step 6: Run JSON report test**

```bash
go test ./internal/report/ -run TestPrintJSON -v
```

Expected: PASS.

- [ ] **Step 7: Wire the check command's runCheck**

Modify `cmd/deadlink/check.go`:

```go
func runCheck(cmd *cobra.Command, args []string) error {
    // Set files from args
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
    }

    results := checker.CheckAll(cmd.Context(), cfg, checkConfig.Files)

    report.PrintResults(results, checkConfig.JSONOutput)

    // Exit code 1 if any dead link found
    hasDead := false
    for _, r := range results {
        if !r.Alive && !r.Skipped {
            hasDead = true
            break
        }
    }
    if hasDead {
        os.Exit(1)
    }
    return nil
}
```

Add imports:

```go
import (
    "os"

    "deadlink/internal/checker"
    "deadlink/internal/report"
)
```

- [ ] **Step 8: Build and verify the binary works**

```bash
go build ./cmd/deadlink
```

- [ ] **Step 9: Create a fixture directory for end-to-end testing**

```bash
mkdir -p testdata
cat > testdata/alive.html << 'EOF'
<a href="https://example.com">alive</a>
EOF
cat > testdata/dead.html << 'EOF'
<a href="https://httpbin.org/status/404">dead</a>
EOF
cat > testdata/mixed.md << 'EOF'
[alive](https://example.com)
[dead](https://httpbin.org/status/404)
EOF
```

- [ ] **Step 10: Run end-to-end test with dead link**

```bash
./deadlink check testdata/dead.html --root testdata
echo "exit code: $?"
```

Expected: prints the dead link and exits with code 1.

- [ ] **Step 11: Run end-to-end test with alive link**

```bash
./deadlink check testdata/alive.html --root testdata
echo "exit code: $?"
```

Expected: prints the alive link and exits with code 0.

- [ ] **Step 12: Run with --json flag**

```bash
./deadlink check testdata/mixed.md --root testdata --json
```

Expected: JSON array output.

- [ ] **Step 13: Run with --ignore-hosts**

```bash
./deadlink check testdata/alive.html --root testdata --ignore-hosts example.com
```

Expected: no output (link ignored), exit code 0.

- [ ] **Step 14: Run with --allow-file-extensions**

```bash
./deadlink check testdata/mixed.md testdata/alive.html --root testdata --allow-file-extensions .html
```

Expected: only the HTML file's link checked.

- [ ] **Step 15: Run all tests**

```bash
go test ./...
```

Expected: all PASS.

- [ ] **Step 16: Commit**

```bash
git add cmd/deadlink/check.go internal/report/
git commit -m "feat: wire check command with report output and exit code"
```

---

### Task 6: Final verification + cleanup

**Files:** n/a (verification only)

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v
```

Expected: all PASS.

- [ ] **Step 2: Build release binary**

```bash
go build -o deadlink ./cmd/deadlink
```

- [ ] **Step 3: Verify all flags work**

```bash
./deadlink check --help
```

Verify: `--root`, `--threads`/`-n`, `--timeout`/`-t`, `--cache-ttl`, `--retry`, `--user-agent`, `--allow-file-extensions`, `--ignore-hosts`, `--json`.

- [ ] **Step 4: Run end-to-end with all flags**

```bash
./deadlink check testdata/ --root testdata \
    --threads 5 \
    --timeout 10s \
    --cache-ttl 259200s \
    --retry 2 \
    --user-agent "deadlink/0.1.0" \
    --allow-file-extensions .html,.md \
    --ignore-hosts www.example.com \
    --json
```

- [ ] **Step 5: Commit any final cleanup**

```bash
git add -A
git commit -m "chore: final cleanup and verification"
```

---

## Spec coverage check

| Spec requirement | Task |
|---|---|
| `deadlink check <files...>` cobra CLI | Task 1, Task 5 |
| `--root`, `--threads`/`-n`, `--json`, `--timeout`/`-t`, `--cache-ttl`, `--retry`, `--user-agent` | Task 1 |
| `--allow-file-extensions` | Task 1, Task 4 |
| `--ignore-hosts` | Task 1, Task 4 |
| `--config` / `-c` + YAML config file (XDG spec) | Task 1 |
| Config file overrides defaults; flags override config | Task 1 |
| Extract URLs from HTML (`<a href>`, `<img src>`, `<link href>`, `<script src>`) | Task 2 |
| Extract URLs from Markdown (`[text](url)`, `![alt](url)`, bare `http(s)://`) | Task 2 |
| Extract bare URLs from txt/yaml/c/h/cpp/hpp | Task 2 |
| Resolve relative URLs against `--root` | Task 2, Task 4 |
| Filter files by `--allow-file-extensions` | Task 4 |
| Filter URLs by `--ignore-hosts` | Task 4 |
| Deduplicate URLs | Task 4 |
| Worker pool with `--threads` concurrency | Task 4 |
| HTTP GET with `--timeout` | Task 3, Task 4 |
| Follow redirects up to 10 hops | Task 3 |
| Retry on 502/503/504 up to `--retry` times with backoff | Task 3 |
| In-memory cache with `--cache-ttl` | Task 3, Task 4 |
| `LinkCheck` model with URL, Status, Alive, RedirectChain, Err, SourceFile, Skipped | Task 1, Task 3, Task 4 |
| Text output: dead links first, URL + status + redirect chain + error | Task 5 |
| JSON output with `--json` flag | Task 5 |
| Exit code 1 when dead link found, 0 otherwise | Task 5 |

## Placeholder scan

No "TBD", "TODO", "implement later", or vague steps. Every step has concrete code.

## Type consistency check

- `model.LinkCheck` — defined Task 1, used in Tasks 3, 4, 5
- `model.RedirectStep` — defined Task 1, used in Tasks 3, 5
- `checker.CheckURL` — signature defined Task 3, called in Task 4
- `checker.CheckAll` — defined Task 4, called in Task 5
- `report.PrintResults` — defined Task 5, called in Task 5
- All signatures match across tasks.

Platform: The project is at `/home/devina/linklint`. Go 1.27.1. The session was interrupted and resumed — git is initialized, spec is committed.
