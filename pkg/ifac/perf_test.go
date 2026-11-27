// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package ifac

import (
	"testing"
)

// TestMaskUnmaskAllocBudget fails if IFAC round-trip allocs grow with payload
// the way the old per-block hmac.New HKDF path did. About 7 of the budget is
// the per-sign heap copy: securemem keys live outside the Go heap and stdlib
// ed25519 needs a heap pointer, so each sign allocates a scoped copy plus a
// fresh FIPS key derivation instead of hitting the stdlib weak-handle cache.
// The race detector adds a few more allocations per sign. The budget covers it.
func TestMaskUnmaskAllocBudget(t *testing.T) {
	id, err := New(0, "budget-net", "budget-pass")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := make([]byte, 2+1024)
	raw[0] = 0x40

	allocs := testing.AllocsPerRun(100, func() {
		masked, err := id.Mask(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, err := id.Unmask(masked); err != nil || !ok {
			t.Fatalf("unmask ok=%v err=%v", ok, err)
		}
	})
	if allocs > 54 {
		t.Fatalf("IFAC mask+unmask(1024) allocs=%.1f want <= 54", allocs)
	}
}

func TestMaskIntoScratchAllocBudget(t *testing.T) {
	id, err := New(0, "budget-net", "budget-pass")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := make([]byte, 2+1024)
	raw[0] = 0x40
	buf := make([]byte, 0, len(raw)+id.Size()+8)

	allocs := testing.AllocsPerRun(100, func() {
		masked, err := id.MaskInto(buf[:0], raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, err := id.Unmask(masked); err != nil || !ok {
			t.Fatalf("unmask ok=%v err=%v", ok, err)
		}
	})
	if allocs > 54 {
		t.Fatalf("IFAC MaskInto+unmask(1024) allocs=%.1f want <= 54", allocs)
	}
}
