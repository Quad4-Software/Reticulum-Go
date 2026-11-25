// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"runtime/debug"
	"testing"
)

func TestApplyDefaultGCPercent(t *testing.T) {
	old := debug.SetGCPercent(42)
	t.Cleanup(func() { debug.SetGCPercent(old) })

	if !ApplyDefaultGCPercent() {
		t.Fatal("ApplyDefaultGCPercent did not apply without GOGC")
	}
	if got := debug.SetGCPercent(42); got != DefaultGCPercent {
		t.Fatalf("GC percent = %d, want %d", got, DefaultGCPercent)
	}
}

func TestApplyDefaultGCPercentHonorsGOGC(t *testing.T) {
	t.Setenv("GOGC", "100")
	old := debug.SetGCPercent(42)
	t.Cleanup(func() { debug.SetGCPercent(old) })

	if ApplyDefaultGCPercent() {
		t.Fatal("ApplyDefaultGCPercent applied despite GOGC")
	}
	if got := debug.SetGCPercent(42); got != 42 {
		t.Fatalf("GC percent = %d, want unchanged 42", got)
	}
}
