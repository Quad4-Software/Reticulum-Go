// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package ifac

import (
	"fmt"
	"testing"
)

func BenchmarkMaskUnmask(b *testing.B) {
	id, err := New(16, "bench-net", "bench-pass")
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	for _, payload := range []int{64, 475, 1024, 4096} {
		b.Run(fmt.Sprintf("Payload-%d", payload), func(b *testing.B) {
			raw := make([]byte, 2+payload)
			raw[0] = 0x40
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				masked, err := id.Mask(raw)
				if err != nil {
					b.Fatal(err)
				}
				if _, ok, err := id.Unmask(masked); err != nil || !ok {
					b.Fatalf("unmask failed: ok=%v err=%v", ok, err)
				}
			}
		})
	}
}

func BenchmarkMaskOnly(b *testing.B) {
	id, err := New(16, "bench-net", "bench-pass")
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	raw := make([]byte, 1026)
	raw[0] = 0x40
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := id.Mask(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmaskOnly(b *testing.B) {
	id, err := New(16, "bench-net", "bench-pass")
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	raw := make([]byte, 1026)
	raw[0] = 0x40
	masked, err := id.Mask(raw)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, err := id.Unmask(masked); err != nil || !ok {
			b.Fatalf("unmask failed: ok=%v err=%v", ok, err)
		}
	}
}

func TestMaskUnmaskGoldenRoundTrip(t *testing.T) {
	id, err := New(16, pythonNetname, pythonNetkey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	raw := mustHex(t, pythonRaw)
	masked, err := id.Mask(raw)
	if err != nil {
		t.Fatalf("Mask: %v", err)
	}
	got, ok, err := id.Unmask(masked)
	if err != nil || !ok {
		t.Fatalf("Unmask: ok=%v err=%v", ok, err)
	}
	if string(got) != string(raw) {
		t.Fatalf("round trip mismatch")
	}
}
