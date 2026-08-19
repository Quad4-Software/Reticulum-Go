// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"quad4/reticulum-go/pkg/cli"
	"quad4/reticulum-go/pkg/term"
)

// daemonOptions holds parsed daemon flags.
type daemonOptions struct {
	ConfigPath string
	DebugLevel int // -1 means unset
	JSONLogs   bool
}

// parseDaemonFlags handles flags when the daemon subcommand (or bare binary) is selected.
func parseDaemonFlags(args []string) (opts daemonOptions, run bool, exitCode int) {
	opts.DebugLevel = -1
	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help" || arg == "-?":
			_ = cli.Main([]string{"--help"}, cli.Options{
				Stdout:      os.Stdout,
				VersionLine: versionLine(),
			})
			return opts, false, 0
		case arg == "-v" || arg == "--version":
			printVersion(os.Stdout)
			return opts, false, 0
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, term.Red(os.Stderr, arg+" requires a path"))
				return opts, false, 2
			}
			i++
			opts.ConfigPath = args[i]
		case strings.HasPrefix(arg, "-config="):
			opts.ConfigPath = strings.TrimPrefix(arg, "-config=")
		case strings.HasPrefix(arg, "--config="):
			opts.ConfigPath = strings.TrimPrefix(arg, "--config=")
		case arg == "-debug" || arg == "--debug":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, term.Red(os.Stderr, arg+" requires a level 0-7"))
				return opts, false, 2
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 || n > 7 {
				fmt.Fprintln(os.Stderr, term.Red(os.Stderr, fmt.Sprintf("invalid debug level %q", args[i])))
				return opts, false, 2
			}
			opts.DebugLevel = n
		case strings.HasPrefix(arg, "-debug="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "-debug="))
			if err != nil || n < 0 || n > 7 {
				fmt.Fprintln(os.Stderr, term.Red(os.Stderr, "invalid debug level"))
				return opts, false, 2
			}
			opts.DebugLevel = n
		case strings.HasPrefix(arg, "--debug="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--debug="))
			if err != nil || n < 0 || n > 7 {
				fmt.Fprintln(os.Stderr, term.Red(os.Stderr, "invalid debug level"))
				return opts, false, 2
			}
			opts.DebugLevel = n
		case arg == "-json-logs" || arg == "--json-logs":
			opts.JSONLogs = true
		default:
			fmt.Fprintln(os.Stderr, term.Red(os.Stderr, fmt.Sprintf("unknown daemon arguments: %q", arg)))
			return opts, false, 2
		}
		i++
	}
	return opts, true, 0
}
