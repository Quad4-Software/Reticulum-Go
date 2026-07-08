// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pathutil

import "os"

// ConfigHomeDir returns a base directory for Reticulum config and storage.
// WASI and some test environments do not define a home directory; fall back
// through $HOME, $XDG_CONFIG_HOME, the working directory, then ".".
func ConfigHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil && usableConfigRoot(home) {
		return home
	}
	if h := os.Getenv("HOME"); usableConfigRoot(h) {
		return h
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	if wd, err := os.Getwd(); err == nil && usableConfigRoot(wd) {
		return wd
	}
	return "."
}

func usableConfigRoot(s string) bool {
	return s != "" && s != "/"
}
