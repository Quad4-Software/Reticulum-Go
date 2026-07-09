// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"os"

	"quad4/reticulum-go/pkg/cli"
)

// parseDaemonFlags handles flags when the daemon subcommand (or bare binary) is selected.
func parseDaemonFlags(args []string) (run bool, exitCode int) {
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "-?":
			_ = cli.Main([]string{"--help"}, cli.Options{
				Stdout:      os.Stdout,
				VersionLine: versionLine(),
			})
			return false, 0
		case "-v", "--version":
			printVersion(os.Stdout)
			return false, 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unknown daemon arguments: %q\n", args)
		return false, 2
	}
	return true, 0
}
