package main

import (
	"fmt"
	"os"

	"github.com/Phisys-Ltd/phis-host/internal/version"
)

func printUsage() {
	fmt.Println(`phis-host

Usage:
  phis-host version
  phis-host help
`)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		printUsage()
		return
	}

	switch args[0] {
	case "version":
		fmt.Println(version.String())
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
}
