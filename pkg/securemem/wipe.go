// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package securemem

import "runtime"

// WipeBytes overwrites b with zeros. Prefer for temporary key staging.
func WipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
