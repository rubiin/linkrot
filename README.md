# linkrot

![CI](https://github.com/rubiin/linkrot/actions/workflows/ci.yml/badge.svg)
![Release](https://img.shields.io/github/v/release/rubiin/linkrot)
![Go Report Card](https://goreportcard.com/badge/github.com/rubiin/linkrot)
![GolangCI](https://golangci.com/badges/github.com/rubiin/linkrot.svg)
![License](https://img.shields.io/github/license/rubiin/linkrot)

A fast link checker for local files, written in Go. Reads HTML, Markdown, and plain-text files, extracts the URLs inside them, and checks each one over HTTP — concurrently, with retries, caching, and per-host filtering.

## Install

```bash
go install linkrot/cmd/linkrot@latest
```

## Usage

```bash
linkrot check [flags] <files...>
```

```bash
# Check links in one file
linkrot check README.md

# Check several files with 20 concurrent requests and JSON output
linkrot check README.md docs/*.md -n 20 --json

# Ignore a host and only parse certain file types
linkrot check docs/* --ignore-hosts www.example.com --allow-file-extensions .md,.txt
```

Exit code is `1` if any dead link was found, `0` otherwise — so it drops straight into CI.

Text output ends with a summary line (`Summary: X alive, Y dead`); disable it with `--summary=false` or `summary: false` in the config file. The summary is never added to `--json` output.

Dead links print in red, alive ones in green. With the default `--color auto`, color is used only when stdout is a terminal; `NO_COLOR` disables it everywhere. JSON output is never colorized.

## Shell completions

Tab completion for commands and flags is built in via the `completion` command:

**bash**

```bash
source <(linkrot completion bash)
```

To load it in every shell, add that line to `~/.bashrc`, or install the script:

```bash
linkrot completion bash > ~/.local/share/bash-completion/completions/linkrot
```

**zsh**

```bash
source <(linkrot completion zsh)
# or install:
linkrot completion zsh > "${fpath[1]}/_linkrot"
```

**fish**

```fish
linkrot completion fish | source
# or install:
linkrot completion fish > ~/.config/fish/completions/linkrot.fish
```

**powershell**

```powershell
linkrot completion powershell | Out-String | Invoke-Expression
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | Base directory for resolving relative links |
| `-n, --threads` | `10` | Concurrent HTTP checks |
| `-t, --timeout` | `10s` | Per-request timeout |
| `--cache-ttl` | `72h` (259200s) | How long a check result is reused |
| `--retry` | `2` | Retries on 502/503/504 (and connection errors) |
| `--user-agent` | `linkrot/0.1.0` | User-Agent header |
| `--allow-file-extensions` | all | Only parse files with these extensions (e.g. `.txt,.yaml,.c`) |
| `--ignore-hosts` | none | Skip URLs with these hosts (comma-separated) |
| `--json` | off | Output a JSON array instead of text |
| `--summary` | on | Append a `Summary: X alive, Y dead` line to text output |
| `--color` | `auto` | When to colorize: `auto`, `always`, or `never` |
| `-c, --config` | XDG path | Path to a YAML config file |

## Config file

All flags can also be set in a YAML config file. It is read from `$XDG_CONFIG_HOME/linkrot/config.yaml` (or `~/.config/linkrot/config.yaml`), or from an explicit path via `--config`. Command-line flags take precedence over the config file.

```yaml
root: "."
threads: 10
timeout: 10s
cache-ttl: 259200s
retry: 2
user-agent: "linkrot/1.0"
allow-file-extensions: [".md", ".html"]
ignore-hosts: ["www.example.com"]
json: false
summary: true
color: auto
```

## Output

Text (default): dead links first, each with its final status, transport error, and redirect chain:

```
https://example.com/gone → DEAD (404)
https://example.com/moved → DEAD (301) [redirect: 301 https://example.com/old -> https://example.com/deeper]
https://example.com → 200
Summary: 1 alive, 2 dead
```

`--json`: the same results as a JSON array with `url`, `status`, `alive`, `redirect_chain`, `error`, and `source_file`.

## Development

This project uses [just](https://github.com/casey/just):

```bash
just build       # build the binary
just test        # run tests
just lint        # vet + gofmt check
just self-check  # run linkrot against this README
```

Or without just:

```bash
go test ./...        # unit tests
go test -race ./...  # with the race detector
go build ./cmd/linkrot
```

## Out of scope

By design there is no recursive site crawling (no `--full-site-check`, no robots.txt handling): the tool checks the URLs found in the files you give it.
