default:
    @just --list

# Build the linkrot binary
build:
    go build -ldflags "-s -w" -o linkrot ./cmd/linkrot

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
    gofmt -w .

# Lint: vet + fmt check + tests
lint:
    go vet ./...
    test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)

# Run linkrot against the repo's own docs
self-check: build
    ./linkrot check README.md

# Clean build artifacts
clean:
    rm -f linkrot

