// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !windows

package storage

import "os"

// syncDir flushes the directory entry so a rename survives a crash.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil { // #nosec G304 -- dir is an internal storage path fsynced for durability, never a user-controlled read target
		_ = d.Sync()
		_ = d.Close()
	}
}
