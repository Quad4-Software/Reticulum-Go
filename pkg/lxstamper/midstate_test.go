// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package lxstamper

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestMidstateMatchesFullHash(t *testing.T) {
	for _, n := range []int{0, 1, 63, 64, 65, 127, 128, 200, 5120} {
		prefix := bytes.Repeat([]byte{0xA5}, n)
		stamp := bytes.Repeat([]byte{0x3C}, StampSize)
		ms, err := midstateOfPrefix(prefix)
		if err != nil {
			t.Fatalf("n=%d midstate: %v", n, err)
		}
		got := hashMidstateStamp(ms, stamp)
		want := sha256.Sum256(append(append([]byte{}, prefix...), stamp...))
		if got != want {
			t.Fatalf("n=%d midstate hash mismatch\ngot  %x\nwant %x", n, got, want)
		}
		viaHash, err := hashFromMidstateStamp(ms, stamp)
		if err != nil {
			t.Fatalf("n=%d hashFromMidstate: %v", n, err)
		}
		if viaHash != want {
			t.Fatalf("n=%d marshal midstate hash mismatch", n)
		}
	}
}
