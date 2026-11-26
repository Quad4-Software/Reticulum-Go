// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"flag"
	"io"
)

// DaemonFlagValues holds the values bound by NewDaemonFlagSet.
type DaemonFlagValues struct {
	ConfigPath string
	DebugLevel int
	JSONLogs   bool
	Help       bool
	Version    bool
}

// NewDaemonFlagSet builds the daemon command flag set.
func NewDaemonFlagSet() (*flag.FlagSet, *DaemonFlagValues) {
	values := &DaemonFlagValues{DebugLevel: -1}
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&values.ConfigPath, "config", "", "path to config file or directory")
	fs.IntVar(&values.DebugLevel, "debug", -1, "debug level 0-7")
	fs.BoolVar(&values.JSONLogs, "json-logs", false, "write logs as JSON")
	fs.BoolVar(&values.Help, "h", false, "print help and exit")
	fs.BoolVar(&values.Help, "help", false, "print help and exit")
	fs.BoolVar(&values.Help, "?", false, "print help and exit")
	fs.BoolVar(&values.Version, "v", false, "print version and exit")
	fs.BoolVar(&values.Version, "version", false, "print version and exit")
	return fs, values
}
