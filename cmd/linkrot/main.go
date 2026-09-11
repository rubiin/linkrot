package main

// version is overridden at release build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	Execute()
}
