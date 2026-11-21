// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !linux

package securemem

import "testing"

func readRSS(t *testing.T) int64 {
	t.Helper()
	return 0
}
