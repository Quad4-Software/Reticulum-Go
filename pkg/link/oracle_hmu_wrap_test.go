// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/resource"
)

func TestHMUSegmentIndexWrapDoesNotOverwriteSlotZero(t *testing.T) {
	rx := &incomingResourceAsm{
		hashmapSegLen: 8,
		totalParts:    16,
		partSlots:     make([][]byte, 16),
		mapHashes:     make([][]byte, 16),
	}

	marker := bytes.Repeat([]byte{0xAA}, resource.MapHashLen)
	hashmapBytes := make([]byte, resource.MapHashLen*4)
	copy(hashmapBytes, marker)

	added := rx.applyHashmapSegment(1<<61, hashmapBytes)
	if added != 0 {
		t.Fatalf("wrapped segment applied %d entries", added)
	}
	if rx.mapHashes[0] != nil {
		t.Fatalf("slot 0 overwritten by wrapping HMU segment: %x", rx.mapHashes[0])
	}
}
