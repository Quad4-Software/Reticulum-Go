// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasm || haiku || plan9

package securemem

// Heap fallback for targets without guarded anonymous mapping support
// (js/wasm, haiku, plan9). Secrets still get canary layout and wiping.

func mapSecret(dataSize int) (mapping, usable []byte, err error) {
	b := make([]byte, dataSize)
	return b, b, nil
}

func unmapSecret(mapping []byte) {}

func protectRange(b []byte, rw bool) error { return nil }

func lockRange(b []byte) error { return errNoLock }

func unlockRange(b []byte) {}

const isHeapFallback = true
