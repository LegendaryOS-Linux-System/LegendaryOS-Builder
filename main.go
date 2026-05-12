package main

import (
	"github.com/legendaryos/builder/src/cli"
)

// Set by -ldflags at build time
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	cli.Version = Version
	cli.Commit = Commit
	cli.BuildDate = BuildDate
	cli.Execute()
}
