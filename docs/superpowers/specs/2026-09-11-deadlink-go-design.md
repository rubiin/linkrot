# deadlink Go CLI — Design Spec

> 1-1 capability replacement for the Python `deadlink`/`deadlinks` package (butuzov/deadlinks), rewritten in Go with a Go-idiomatic CLI surface using cobra.

## Goal

A CLI tool that reads links from local HTML/Markdown files, resolves them relative to a root directory, then checks each link by issuing real HTTP requests — reporting which are alive and which are dead.

## Command surface

```
deadlink check [flags] <files...>
```

Flags:

- `--root` (default `.`) — base directory for resolving relative links found in files
- `--threads` / `-n` (default `10`) — concurrency for HTTP checks
- `--json` — output as JSON array instead of structured text
- `--timeout` / `-t` (default `10s`) — per-request timeout
- `--cache-ttl` (default `259200s` / 3 days) — cache TTL: previously-checked URLs within this window are reused without re-fetching
- `--retry` (default `2`) — number of retries on 502/503/504
- `--user-agent` — optional; defaults to `deadlink/1.0`
- `--allow-file-extensions` (default: all) — only parse files with these extensions (e.g. `txt,yaml,c`); if set, non-matching files are skipped
- `--ignore-hosts` — skip HTTP checks for URLs whose host is in this comma-separated list
- `--config` / `-c` (default: XDG config path) — path to YAML config file; if not specified, reads from XDG config dir (`$XDG_CONFIG_HOME/deadlink/config.yaml` or `~/.config/deadlink/config.yaml`)
- `--help` / `-h`

Config file (YAML) supports the same keys as flags. Flag values override config file values. If neither is set, defaults apply.

Exit code: `1` if any dead link found, `0` otherwise.

## Pipeline

1. **Filter files** — if `--allow-file-extensions` is set, skip files whose extension is not in the list.
2. **Parse** — read each input file, extract HTTP(S) URLs. For HTML, parse `<a href>`, `<img src>`, `<link href>`, `<script src>`, etc. For Markdown, extract URLs from `[text](url)`, `![alt](url)`, and bare `http(s)://` links. For plain text and other allowed extensions, extract bare `http(s)://` URLs. Resolve relative URLs against `--root`.
3. **Filter by host** — drop any URL whose host matches `--ignore-hosts` (no check performed, not reported).
4. **Deduplicate** — same URL checked once.
5. **Check** — fan out to a worker pool of `--threads` goroutines. Each worker: check in-memory cache (keyed by URL, valid for `--cache-ttl`); on miss, `GET` the URL (with `--timeout`), follow redirects up to a limit of 10, retry on 502/503/504 up to `--retry` times with exponential backoff, then store result in cache.
6. **Collect** — each result is a struct: `{URL, Status, Alive, RedirectChain, Err, SourceFile}`.
7. **Report** — print results. Dead links first (or all, with alive/dead marker). With `--json`, emit a JSON array.

## Key types

```go
type LinkCheck struct {
    URL           string
    Status        int    // final HTTP status, 0 if skipped/error
    Alive         bool
    RedirectChain []RedirectStep
    Err           string // if fetch failed non-HTTP
    SourceFile    string // which input file the link came from
    Skipped       bool   // true when host matched --ignore-hosts
}

type RedirectStep struct {
    URL    string
    Status int
}
```

## Modules / files

- `cmd/deadlink/main.go` — cobra CLI entrypoint, wiring
- `cmd/deadlink/config.go` — config file loading (XDG spec: `$XDG_CONFIG_HOME/deadlink/config.yaml` or `~/.config/deadlink/config.yaml`), merging with flag values
- `internal/extract/` — URL extraction from HTML (via `golang.org/x/net/html`) and Markdown (regex-based)
- `internal/checker/` — worker pool, HTTP fetch with redirect following + retry logic
- `internal/report/` — text and JSON output formatting
- `internal/model/` — shared types (`LinkCheck`, `RedirectStep`)

Config file schema (YAML):

```yaml
root: "."
threads: 10
json: false
timeout: 10s
cache-ttl: 259200s
retry: 2
user-agent: "deadlink/1.0"
allow-file-extensions: []
ignore-hosts: []
```

Flag values always override config file values. If `--config` is not specified, the tool reads the default XDG config path. If no config file exists at that path, defaults are used.

## Dependencies

- Standard library for HTTP, concurrency, JSON, CLI wiring
- `github.com/spf13/cobra` for CLI
- `golang.org/x/net/html` for robust HTML parsing
- No external Markdown parser — regex extraction for URLs only

## Out of scope

- Recursive crawling of a remote site (no full-site-check, no robots.txt)
- Checking links inside non-HTML/Markdown/allowed file types (PDF, Word) — easy to add later

## Testing approach

- Extractor tests: feed sample HTML/MD strings, assert extracted URL sets.
- Checker tests: with `net/http/httptest` serving known status codes and redirect chains; verify retry behavior and timeout.
- End-to-end: run the binary against a small fixture directory of HTML/MD files with known links; assert output and exit code.
