// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build tinygo

package selfcheck

import "context"

func ChildExitCode() (code int, handled bool) {
	return 0, false
}

func Run(ctx context.Context, opts Options) Report {
	_ = ctx
	_ = opts
	return Report{
		GOOS:      goos(),
		GOARCH:    goarch(),
		GoVersion: goVersion(),
		Results: []Result{
			result("tinygo", SeveritySkip, "self-check is not supported on TinyGo"),
		},
	}
}
