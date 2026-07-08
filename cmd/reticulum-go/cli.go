// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `reticulum-go - Reticulum network stack daemon (Go)

Usage:
  reticulum-go [flags]

Flags:
  -h, --help       Print this help and exit
  -v, --version    Print version and exit

Configuration is loaded from ~/.reticulum-go/config (created on first run).
`)
}

// parseCLI handles top-level flags. It returns true when the daemon should start.
func parseCLI(args []string) (runDaemon bool, exitCode int) {
	for _, arg := range args {
		switch arg {
		case "-h", "-?":
			printHelp(os.Stdout)
			return false, 0
		}
	}

	fs := flag.NewFlagSet("reticulum-go", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	showVersion := fs.Bool("version", false, "print version and exit")
	showHelp := fs.Bool("help", false, "print help and exit")

	if err := fs.Parse(args); err != nil {
		return false, 2
	}
	if *showHelp {
		printHelp(os.Stdout)
		return false, 0
	}
	if *showVersion {
		printVersion(os.Stdout)
		return false, 0
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unknown arguments: %q\n\n", fs.Args())
		printHelp(os.Stderr)
		return false, 2
	}
	return true, 0
}
