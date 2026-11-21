// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build (darwin || freebsd || openbsd || netbsd || dragonfly || solaris || illumos || aix || ios) && !wasm

package securemem

import (
	"golang.org/x/sys/unix"
)

// HardenProcess disables core dumps so secrets cannot escape through a
// core file. Best effort.
func HardenProcess() {
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &lim); err == nil && lim.Cur != 0 {
		lim.Cur = 0
		_ = unix.Setrlimit(unix.RLIMIT_CORE, &lim)
	}
}
