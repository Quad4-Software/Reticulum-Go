// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build js || wasm || haiku || plan9 || windows

package securemem

// HardenProcess is a no-op on platforms without dumpability controls.
func HardenProcess() {}
