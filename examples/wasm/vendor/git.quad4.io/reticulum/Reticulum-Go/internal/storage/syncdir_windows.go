// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build windows

package storage

// syncDir is a no-op on Windows, which does not support fsync on directory
// handles. The file sync before rename still protects file contents.
func syncDir(string) {}
