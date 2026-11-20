// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows && !js

package main

import (
	"os"
	"os/signal"
	"syscall"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

// watchHeapDumpSignal writes a bounded heap profile to dir on SIGUSR1 so
// an operator can grab one from a production node without restarting it.
func watchHeapDumpSignal(dir string) {
	if dir == "" {
		return
	}
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGUSR1)
		for range ch {
			path, err := writeHeapProfileBounded(dir)
			if err != nil {
				debug.Log(debug.DebugError, "SIGUSR1 heap dump failed", "error", err)
				continue
			}
			debug.Log(debug.DebugInfo, "Heap profile written", "path", path)
		}
	}()
}
