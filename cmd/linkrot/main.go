package main

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	Execute()
}
