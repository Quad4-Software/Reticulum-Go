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
	fd := f.Fd()
	maxInt := int(^uint(0) >> 1)
	if fd > uintptr(maxInt) {
		return false
	}
	// #nosec G115 -- fd is bounded above by max int
	return xterm.IsTerminal(int(fd))
}
