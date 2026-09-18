// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package sample

import "testing"

func TestLessThan(t *testing.T) {
	if LessThan(2, 1) {
		t.Fatal("expected false")
	}
	if !LessThan(1, 2) {
		t.Fatal("expected true")
	}
}
