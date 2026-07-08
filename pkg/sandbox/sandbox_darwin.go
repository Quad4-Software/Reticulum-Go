// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build darwin

package sandbox

import (
	"golang.org/x/sys/unix"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

func applyPlatform(cfg *common.ReticulumConfig) error {
	if err := setResourceLimits(); err != nil {
		debug.Log(debug.DebugError, "Setrlimit failed", "error", err)
	}

	debug.Log(debug.DebugInfo, "Sandbox applied", "platform", "darwin")
	return nil
}

func setResourceLimits() error {
	const maxFDs = 65536
	if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: maxFDs, Max: maxFDs}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_NOFILE failed", "error", err)
	}

	const memLimit = 2 << 30 // 2 GiB
	if err := unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: memLimit, Max: memLimit}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_AS failed", "error", err)
	}

	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_CORE failed", "error", err)
	}

	const stackLimit = 8 << 20 // 8 MiB
	if err := unix.Setrlimit(unix.RLIMIT_STACK, &unix.Rlimit{Cur: stackLimit, Max: unix.RLIM_INFINITY}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_STACK failed", "error", err)
	}

	const procLimit = 4096
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: procLimit, Max: procLimit}); err != nil {
		debug.Log(debug.DebugError, "RLIMIT_NPROC failed", "error", err)
	}

	return nil
}
