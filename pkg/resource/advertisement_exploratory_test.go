// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/pbt/pkg/pbt"
)

func TestUnpackRejectsNegativeSizesAndParts(t *testing.T) {
	cases := []struct {
		name string
		dict map[string]any
	}{
		{"neg_transfer", map[string]any{"t": int64(-1), "d": int64(1), "n": 1, "h": make([]byte, 32)}},
		{"neg_data", map[string]any{"t": int64(1), "d": int64(-1), "n": 1, "h": make([]byte, 32)}},
		{"neg_parts", map[string]any{"t": int64(1), "d": int64(1), "n": int64(-1), "h": make([]byte, 32)}},
		{"bad_hash_len", map[string]any{"t": int64(1), "d": int64(1), "n": 1, "h": []byte{1, 2, 3}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := msgpack.Marshal(tc.dict)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := UnpackResourceAdvertisement(b); err == nil {
				t.Fatal("expected unpack error")
			}
		})
	}
}

// TestExploratoryUnpackAllowsOversizedTransfer checks that unpack accepts large
// transfer sizes and leaves rejection to link accept-time checks.
func TestExploratoryUnpackAllowsOversizedTransfer(t *testing.T) {
	dict := map[string]any{
		"t": int64(MaxEfficientSize) + 8192,
		"d": int64(MaxEfficientSize) + 8192,
		"n": 1,
		"h": make([]byte, 32),
		"r": []byte{1, 2, 3, 4},
		"m": make([]byte, MapHashLen),
		"f": 0,
	}
	b, err := msgpack.Marshal(dict)
	if err != nil {
		t.Fatal(err)
	}
	adv, err := UnpackResourceAdvertisement(b)
	if err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if adv.TransferSize <= int64(MaxEfficientSize)+4096 {
		t.Fatalf("fixture transfer_size %d not past link reject bound", adv.TransferSize)
	}
}

func TestWireFlagsFromAnyRejectsUnknownType(t *testing.T) {
	if _, err := wireFlagsFromAny(1.5); err == nil {
		t.Fatal("expected error for float flags")
	}
}

func TestPBTAdvertisementFlagsRoundTrip(t *testing.T) {
	flag := pbt.IntRange(0, 63)
	prop := pbt.ForAll(
		"flags bits match decoded booleans",
		flag,
		func(f int) bool {
			ra := &ResourceAdvertisement{
				TransferSize:  64,
				DataSize:      64,
				Parts:         1,
				Hash:          bytes.Repeat([]byte{0x11}, 32),
				RandomHash:    []byte{1, 2, 3, 4},
				OriginalHash:  bytes.Repeat([]byte{0x22}, 32),
				SegmentIndex:  0,
				TotalSegments: 1,
				Hashmap:       make([]byte, MapHashLen),
				Flags:         byte(f),
			}
			packed, err := ra.Pack(0, 500)
			if err != nil {
				return false
			}
			got, err := UnpackResourceAdvertisement(packed)
			if err != nil {
				return false
			}
			return got.Encrypted == (got.Flags&AdvFlagEncrypted != 0) &&
				got.Compressed == (got.Flags&AdvFlagCompressed != 0) &&
				got.Split == (got.Flags&AdvFlagSplit != 0) &&
				got.IsRequest == (got.Flags&AdvFlagIsRequest != 0) &&
				got.IsResponse == (got.Flags&AdvFlagIsResponse != 0) &&
				got.HasMetadata == (got.Flags&AdvFlagHasMetadata != 0)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(64), pbt.WithSeed(77))
}
