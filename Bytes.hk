package main

import (
	"github.com/legendaryos/builder/src/cli"
)

// Version information — set by -ldflags at build time.
// To release v0.6: git tag v0.6 && git push origin v0.6
var (
	Version   = "v0.6"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	cli.Version   = Version
	cli.Commit    = Commit
	cli.BuildDate = BuildDate
	cli.Execute()
}
