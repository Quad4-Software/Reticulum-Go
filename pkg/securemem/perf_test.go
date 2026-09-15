// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package securemem

import (
	"testing"
)

func BenchmarkBufBytes(b *testing.B) {
	buf, err := New(32)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()
	_ = buf.CopyFrom(make([]byte, 32))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.Bytes()
	}
}

func BenchmarkBufCopyOut(b *testing.B) {
	buf, err := New(32)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()
	_ = buf.CopyFrom(make([]byte, 32))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := buf.CopyOut()
		WipeBytes(out)
	}
}

func TestBufBytesAllocBudget(t *testing.T) {
	buf, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Close()
	_ = buf.CopyFrom(make([]byte, 32))
	allocs := testing.AllocsPerRun(1000, func() {
		_ = buf.Bytes()
	})
	if allocs > 0 {
		t.Fatalf("Buf.Bytes allocs=%.1f want 0", allocs)
	}
}

func TestBufCopyOutAllocBudget(t *testing.T) {
	buf, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Close()
	_ = buf.CopyFrom(make([]byte, 32))
	allocs := testing.AllocsPerRun(1000, func() {
		out := buf.CopyOut()
		WipeBytes(out)
	})
	// One slice for the copy.
	if allocs > 1 {
		t.Fatalf("Buf.CopyOut allocs=%.1f want <= 1", allocs)
	}
}
