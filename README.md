# linkrot


[![CI](https://github.com/rubiin/linkrot/actions/workflows/ci.yml/badge.svg)](https://github.com/rubiin/linkrot/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rubiin/linkrot.svg)](https://pkg.go.dev/github.com/rubiin/linkrot)
[![Release](https://img.shields.io/github/v/release/rubiin/linkrot)](https://github.com/rubiin/linkrot/releases/latest)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

<img alt="linkrot logo" width="300" alt="logo-Photoroom" src="https://github.com/user-attachments/assets/596e908d-58ae-419c-b0d9-8cfa0bf20ac9" />


linkrot is a fast CLI link checker for local files. It reads HTML, Markdown, and plain-text files, extracts every URL, and checks each one over HTTP — concurrently, with retries, caching, and per-host filtering.

- **Concurrency** — check many links in parallel with `-n`.
- **Retries** — automatic retries on 502/503/504 and on connection errors.
- **Cache** — cached results survive across runs for a configurable TTL.
- **Filters** — skip whole hosts or limit parsing to certain file extensions.
- **CI-friendly** — exit code `1` when any dead link is found, `0` otherwise.
- **Shell completions** — built-in `linkrot completion` for bash, zsh, fish, and PowerShell.

## Install

```bash
go install linkrot/cmd/linkrot@latest
```

This installs `linkrot` to `$GOPATH/bin` (or `$HOME/go/bin`). Make sure that directory is on your `PATH`.

**Arch Linux (AUR)**

```bash
yay -S linkrot-bin
```

Or with any other AUR helper:

```bash
paru -S linkrot-bin
```

## Quick start

Check a single file:

```bash
linkrot check README.md
```

Check several files concurrently and output JSON:

```bash
linkrot check README.md docs/*.md -n 20 --json
```

Ignore a host and restrict parsing to specific extensions:

```bash
linkrot check docs/* --ignore-hosts www.example.com --allow-file-extensions .md,.txt
```

## Exit code

The exit code is `1` if any dead link was found, `0` otherwise. This makes linkrot suitable for CI pipelines.

## Output

By default, results are printed as text with dead links first. Each dead link shows its final status, the transport error, and the redirect chain if one exists. Alive links show their HTTP status.

```
https://example.com/gone → DEAD (404)
https://example.com/moved → DEAD (301) [redirect: 301 https://example.com/old -> https://example.com/deeper]
https://example.com → 200
Summary: 1 alive, 2 dead
```

Dead links are shown in red, alive links in green.

## Flags

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | Base directory for resolving relative links. |
| `-n, --threads` | `10` | Number of concurrent HTTP checks. |
| `-t, --timeout` | `10s` | Per-request HTTP timeout. |
| `--cache-ttl` | `72h` | How long a successful check result is reused. |
| `--retry` | `2` | Retries on 502/503/504 and connection errors. |
| `--user-agent` | `linkrot/0.1.0` | Value of the `User-Agent` header. |
| `--allow-file-extensions` | all | Only parse files with these extensions (e.g. `.txt,.yaml,.c`). |
| `--ignore-hosts` | none | Skip URLs whose host matches any of these (comma-separated). |
| `--json` | off | Output a JSON array instead of text. |
| `--summary` | on | Append a `Summary: X alive, Y dead` line to text output. |
| `--color` | `auto` | When to colorize: `auto`, `always`, or `never`. |
| `-c, --config` | XDG path | Path to a YAML config file. |

### Color

With `--color auto` (the default), colors are used only when stdout is a terminal. Set the `NO_COLOR` environment variable to disable colors everywhere. JSON output is never colorized.

### Summary

Text output ends with a summary line (`Summary: X alive, Y dead`). Disable it with `--summary=false` or `summary: false` in the config file. The `--json` output never includes the summary.

## Shell completions

Tab completion for commands and flags is built in via the `completion` command.

**Bash**

```bash
source <(linkrot completion bash)
```

To load it in every shell, add that line to `~/.bashrc`, or install the script:

```bash
linkrot completion bash > ~/.local/share/bash-completion/completions/linkrot
```

**Zsh**

```bash
source <(linkrot completion zsh)
# or install:
linkrot completion zsh > "${fpath[1]}/_linkrot"
```

**Fish**

```fish
linkrot completion fish | source
# or install:
linkrot completion fish > ~/.config/fish/completions/linkrot.fish
```

**PowerShell**

```powershell
linkrot completion powershell | Out-String | Invoke-Expression
```

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

## JSON output

When `--json` is set, results are emitted as a JSON array. Each entry includes:

- `url` — the URL that was checked.
- `status` — the final HTTP status code, or `0` if the request failed.
- `alive` — whether the URL was reachable.
- `redirect_chain` — the sequence of redirects followed, if any.
- `error` — the transport error message, if any.
- `source_file` — the file the URL was extracted from.

## Development

This project uses [just](https://github.com/casey/just) for common tasks:

```bash
just build       # build the binary
just test        # run tests
just lint        # vet + gofmt check
just self-check  # run linkrot against this README
```

**just** recipes:

```bash
just build       # build the binary
just test        # run tests
just test-race   # run tests with the race detector
just lint        # golangci-lint + format check
just vet         # go vet
just self-check  # run linkrot against this README
just completions shell=bash   # print completion script for a shell
just clean       # remove built artifacts
```

Or without just:

```bash
go test ./...        # unit tests
go test -race ./...  # with the race detector
go build ./cmd/linkrot
```

## Design notes

linkrot checks the URLs found in the files you give it. It does **not** crawl a site recursively and does not read `robots.txt`. There is no `--full-site-check`; pass the files you want checked explicitly.

The tool retries on transient failures (502/503/504 and connection errors), and successful results are cached for the configured TTL. Failed results are intentionally not cached, so transient failures are retried on the next run.

## License

[GPL-3.0](./LICENSE)

Made with ❤️ for opensource.

