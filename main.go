package main

import "lsm/cmd"

// version is injected at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
