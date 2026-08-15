// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package selfcheck

import (
	"runtime"
	"runtime/debug"
)

func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }

func goVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.GoVersion != "" {
		return bi.GoVersion
	}
	return runtime.Version()
}
