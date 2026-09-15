// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !unix

package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

func notifyShellSignals(sig chan os.Signal) {
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
}

func shellSignalWinch(os.Signal, func()) bool {
	return false
}

func startResizePoll(done <-chan struct{}, send func()) {
	if send == nil {
		return
	}
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		lastR, lastC := ttySize()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				r, c := ttySize()
				if r == lastR && c == lastC {
					continue
				}
				lastR, lastC = r, c
				send()
			}
		}
	}()
}
