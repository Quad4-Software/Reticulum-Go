// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"

	"quad4/reticulum-go/pkg/resource"
)

// Regression: a RESOURCE_HMU segment index parsed from the wire (attacker
// controlled, see handleResourceHashmapUpdate -> wireInt(update[0])) must
// never index rx.mapHashes/partSlots out of bounds. A negative segment must
// be rejected rather than reaching a negative slice index.
func TestOracleHMUNegativeSegmentDoesNotPanic(t *testing.T) {
	rx := &incomingResourceAsm{
		hashmapSegLen: 8,
		totalParts:    16,
		partSlots:     make([][]byte, 16),
		mapHashes:     make([][]byte, 16),
	}

	hashmapBytes := make([]byte, resource.MapHashLen*4)

	added := rx.applyHashmapSegment(-1000000, hashmapBytes)
	if added != 0 {
		t.Fatalf("expected 0 entries applied for negative segment, got %d", added)
	}
	for i, mh := range rx.mapHashes {
		if mh != nil {
			t.Fatalf("mapHashes[%d] was populated by a negative-segment HMU", i)
		}
	}
}

// A very large positive segment must also be rejected rather than reaching
// an out-of-bounds or overflow-wrapped index.
func TestOracleHMUOversizeSegmentDoesNotPanic(t *testing.T) {
	rx := &incomingResourceAsm{
		hashmapSegLen: 8,
		totalParts:    16,
		partSlots:     make([][]byte, 16),
		mapHashes:     make([][]byte, 16),
	}

	hashmapBytes := make([]byte, resource.MapHashLen*4)

	added := rx.applyHashmapSegment(1<<40, hashmapBytes)
	if added != 0 {
		t.Fatalf("expected 0 entries applied for oversize segment, got %d", added)
	}
}
