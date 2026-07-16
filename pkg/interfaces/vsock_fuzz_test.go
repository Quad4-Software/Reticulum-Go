// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux && !js

package interfaces

import (
	"encoding/binary"
	"math"
	"testing"
)

func FuzzParseVSOCKContextID(f *testing.F) {
	f.Add(int(0))
	f.Add(int(1))
	f.Add(int(-1))
	f.Add(int(0x7fffffff))
	f.Fuzz(func(t *testing.T, v int) {
		cid, err := ParseVSOCKContextID(v)
		if v < 0 || uint64(v) > math.MaxUint32 {
			if err == nil {
				t.Fatal("expected error for out-of-range context ID")
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error for %d: %v", v, err)
		}
		if cid != uint32(v) {
			t.Fatalf("cid=%d want %d", cid, uint32(v))
		}
	})
}

func FuzzVSOCKHDLCDecoder(f *testing.F) {
	f.Add([]byte{HDLCFlag, 0x01, 0x02, HDLCFlag})
	f.Add([]byte{HDLCFlag, HDLCEsc, HDLCFlag ^ HDLCEscMask, HDLCFlag})
	var seed [8]byte
	binary.LittleEndian.PutUint64(seed[:], 0xdeadbeef)
	f.Add(seed[:])
	f.Fuzz(func(t *testing.T, data []byte) {
		var n int
		d := newHDLCToggleStreamDecoder(DefaultMTU, func(payload []byte) {
			n++
			if len(payload) > DefaultMTU {
				t.Fatalf("payload larger than MTU: %d", len(payload))
			}
		})
		for i := 0; i < len(data); {
			end := min(i+13, len(data))
			d.feed(data[i:end])
			i = end
		}
		_ = n
	})
}
