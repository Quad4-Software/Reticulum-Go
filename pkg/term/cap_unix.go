// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows

package term

import (
	"os"

	xterm "golang.org/x/term"
)

func prepareColorFile(f *os.File) bool {
	if f == nil {
		return false
	}
	return xterm.IsTerminal(int(f.Fd()))
}
