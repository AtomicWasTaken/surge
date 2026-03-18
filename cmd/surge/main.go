package main

import (
	"fmt"
	"os"

	"github.com/AtomicWasTaken/surge/internal/cli"
)

var version = "dev"
var commit = ""
var date = ""

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Make version info available to the cli package
	cli.SetVersion(version, commit, date)
}
