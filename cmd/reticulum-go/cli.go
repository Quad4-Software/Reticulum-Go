// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"os"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/cli"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/term"
)

// daemonOptions holds parsed daemon flags.
type daemonOptions struct {
	ConfigPath string
	DebugLevel int // -1 means unset
	JSONLogs   bool
}

// parseDaemonFlags handles flags when the daemon subcommand (or bare binary) is selected.
func parseDaemonFlags(args []string) (opts daemonOptions, run bool, exitCode int) {
	fs, values := cli.NewDaemonFlagSet()
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, term.Red(os.Stderr, err.Error()))
		return opts, false, 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintln(os.Stderr, term.Red(os.Stderr, fmt.Sprintf("unknown daemon arguments: %q", fs.Args()[0])))
		return opts, false, 2
	}
	if values.Help {
		_ = cli.Main([]string{"--help"}, cli.Options{
			Stdout:      os.Stdout,
			VersionLine: versionLine(),
		})
		return opts, false, 0
	}
	if values.Version {
		printVersion(os.Stdout)
		return opts, false, 0
	}
	if values.DebugLevel < -1 || values.DebugLevel > 7 {
		fmt.Fprintln(os.Stderr, term.Red(os.Stderr, fmt.Sprintf("invalid debug level %d", values.DebugLevel)))
		return opts, false, 2
	}
	opts.ConfigPath = values.ConfigPath
	opts.DebugLevel = values.DebugLevel
	opts.JSONLogs = values.JSONLogs
	return opts, true, 0
}
