default:
    @just --list

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`

# Build the linkrot binary
build:
    @echo "Building shelf version: {{version}}"
    go build -ldflags "-s -w -X main.version={{version}}" -o shelf ./cmd/linkrot

# Run all tests
test:
    go test ./...

# Run tests with the race detector
test-race:
    go test -race ./...

# Run go vet
vet:
    go vet ./...

# Format code
fmt:
    golangci-lint fmt

# Lint: golangci-lint (uses built-in defaults)
lint:
    golangci-lint run 

# Run linkrot against the repo's own docs
self-check: build
    ./linkrot check README.md

# Print the completion script for a shell (bash, zsh, fish, powershell)
completions shell="bash":
    ./linkrot completion {{shell}}

# Clean build artifacts
clean:
    rm -f linkrot

