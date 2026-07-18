// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package resource

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
)

// FuzzUnpackResourceAdvertisement ensures msgpack advertisement decoding
// never panics on adversarial blobs.
func FuzzUnpackResourceAdvertisement(f *testing.F) {
	ra := &ResourceAdvertisement{
		TransferSize:  32,
		DataSize:      32,
		Parts:         1,
		Hash:          bytes.Repeat([]byte{0x11}, 32),
		RandomHash:    []byte{0xaa, 0xbb, 0xcc, 0xdd},
		OriginalHash:  bytes.Repeat([]byte{0x22}, 32),
		SegmentIndex:  0,
		TotalSegments: 1,
		Hashmap:       make([]byte, MapHashLen),
		Flags:         AdvFlagEncrypted,
	}
	packed, err := ra.Pack(0, 500)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(packed)
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add(bytes.Repeat([]byte{0xff}, 256))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		adv, err := UnpackResourceAdvertisement(data)
		if err != nil {
			return
		}
		if adv == nil {
			t.Fatal("nil advertisement without error")
		}
		_ = IsRequestAdvertisement(data)
		_ = IsResponseAdvertisement(data)
	})
}

func TestPBTResourceAdvertisementPackUnpack(t *testing.T) {
	parts := pbt.IntRange(1, 64)
	prop := pbt.ForAll(
		"resource advertisement pack unpack sizes",
		parts,
		func(n int) bool {
			ra := &ResourceAdvertisement{
				TransferSize:  int64(n * 64),
				DataSize:      int64(n * 64),
				Parts:         n,
				Hash:          bytes.Repeat([]byte{0x33}, 32),
				RandomHash:    []byte{1, 2, 3, 4},
				OriginalHash:  bytes.Repeat([]byte{0x44}, 32),
				SegmentIndex:  0,
				TotalSegments: 1,
				Hashmap:       bytes.Repeat([]byte{0x55}, MapHashLen*n),
			}
			packed, err := ra.Pack(0, 2000)
			if err != nil {
				return false
			}
			got, err := UnpackResourceAdvertisement(packed)
			if err != nil {
				return false
			}
			return got.Parts == ra.Parts &&
				got.TransferSize == ra.TransferSize &&
				got.DataSize == ra.DataSize &&
				bytes.Equal(got.Hash, ra.Hash)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(13))
}
