// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package securemem

import (
	"golang.org/x/sys/unix"
)

// HardenProcess makes the calling process non-dumpable and disables core
// dumps, so secrets cannot escape through a core file. Best effort; a
// kernel or sandbox that refuses either step does not stop the caller.
func HardenProcess() {
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &lim); err == nil && lim.Cur != 0 {
		lim.Cur = 0
		_ = unix.Setrlimit(unix.RLIMIT_CORE, &lim)
	}
}
