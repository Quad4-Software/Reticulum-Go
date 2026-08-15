// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !tinygo

package sandbox

import "golang.org/x/sys/unix"

const execChildNproc = 32

func applyChildRlimits(pid int) {
	if pid <= 0 {
		return
	}
	_ = unix.Prlimit(pid, unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}, nil)
	_ = unix.Prlimit(pid, unix.RLIMIT_NPROC, &unix.Rlimit{Cur: execChildNproc, Max: execChildNproc}, nil)
}
