// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build unix && !wasm && !haiku

package securemem

import (
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"golang.org/x/sys/unix"
)

var mlockDeniedOnce sync.Once

// allocLocked uses a Go heap slice so crypto APIs that register weak handles
// remain valid. Pages are mlock'd when the OS allows it.
func allocLocked(n int) (data []byte, locked bool, native bool, err error) {
	b := make([]byte, n)
	if err := unix.Mlock(b); err != nil {
		mlockDeniedOnce.Do(func() {
			debug.Log(debug.DebugVerbose, "securemem mlock denied, using unlocked heap", "error", err)
		})
		return b, false, false, nil
	}
	return b, true, false, nil
}

func freeLocked(data []byte, locked bool, native bool) error {
	WipeBytes(data)
	if locked && data != nil {
		_ = unix.Munlock(data)
	}
	return nil
}
