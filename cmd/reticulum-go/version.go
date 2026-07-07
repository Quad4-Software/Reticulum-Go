// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

const defaultVersion = "dev"

func versionLine() string {
	version := defaultVersion
	commit := ""
	buildTime := ""

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.time":
				buildTime = s.Value
			}
		}
	}

	line := fmt.Sprintf("reticulum-go %s %s/%s", version, runtime.GOOS, runtime.GOARCH)
	if commit != "" {
		if len(commit) > 12 {
			commit = commit[:12]
		}
		line += " commit=" + commit
	}
	if buildTime != "" {
		line += " built=" + buildTime
	}
	line += fmt.Sprintf(" go=%s", runtime.Version())
	return line
}

func printVersion(w io.Writer) {
	fmt.Fprintln(w, versionLine())
}
