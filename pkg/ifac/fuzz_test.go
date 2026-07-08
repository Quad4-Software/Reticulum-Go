// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package ifac

import (
	"bytes"
	"testing"
)

// FuzzUnmask exercises the Unmask path with arbitrary byte inputs to make
// sure no input causes a panic, out-of-bounds slice or unbounded allocation.
// Verification failures and length errors are expected and ignored.
func FuzzUnmask(f *testing.F) {
	id, err := New(DefaultSize, "fuzz", "fuzz")
	if err != nil {
		f.Fatalf("setup ifac: %v", err)
	}

	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add(bytes.Repeat([]byte{0xff}, 4096))

	for _, sz := range []int{1, 4, 16, 32} {
		raw := bytes.Repeat([]byte{0x55}, 64)
		raw[0] = 0x01
		raw[1] = 0x00
		id.size = sz
		if masked, err := id.Mask(raw); err == nil {
			f.Add(masked)
		}
	}
	id.size = DefaultSize

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip("oversize input")
		}
		_, _, err := id.Unmask(raw)
		_ = err
	})
}

// FuzzMaskRoundTrip masks arbitrary raw packets and confirms that Unmask
// returns the original buffer with ok=true.
func FuzzMaskRoundTrip(f *testing.F) {
	id, err := New(DefaultSize, "fuzz", "fuzz")
	if err != nil {
		f.Fatalf("setup ifac: %v", err)
	}
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add([]byte{0x01, 0x00, 0xff, 0xff, 0xff})
	f.Add(bytes.Repeat([]byte{0x42}, 256))

	f.Fuzz(func(t *testing.T, raw []byte) {
		// Need at least 1 byte of payload past the header so the masked
		// packet is strictly longer than 2 + ifac size. Empty payload

		// packets are not valid Reticulum packets anyway.
		if len(raw) < 3 || len(raw) > 1<<14 {
			t.Skip()
		}
		raw = append([]byte(nil), raw...)
		raw[0] &^= IFACFlag
		masked, err := id.Mask(raw)
		if err != nil {
			t.Fatalf("Mask failed: %v", err)
		}
		got, ok, err := id.Unmask(masked)
		if err != nil {
			t.Fatalf("Unmask error: %v", err)
		}
		if !ok {
			t.Fatalf("Unmask rejected freshly masked packet")
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("round trip mismatch:\n got=%x\nwant=%x", got, raw)
		}
	})
}
