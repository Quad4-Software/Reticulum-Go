// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package securemem

import (
	"bytes"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"
)

// Canary tamper: corrupting a slab cell's canary is detected on the next
// operation and on Close, which still wipes and releases the cell.
func TestCanaryTamperDetected(t *testing.T) {
	b, err := New(32)
	if err != nil {
		t.Fatal(err)
	}
	if b.cell == nil {
		t.Skip("buffer not slab-allocated")
	}
	off := b.cell.idx * b.cell.ar.stride
	b.cell.ar.data[off] ^= 0xff // corrupt canary A inside the arena
	if err := b.CopyFrom(bytes.Repeat([]byte{1}, 32)); err != ErrCanary {
		t.Fatalf("CopyFrom err=%v want ErrCanary", err)
	}
	if err := b.Close(); err != ErrCanary {
		t.Fatalf("Close err=%v want ErrCanary", err)
	}
}

// Canary stays intact through normal use.
func TestCanaryIntact(t *testing.T) {
	b, err := New(64)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.CopyFrom(bytes.Repeat([]byte{0x5a}, 64)); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close err=%v", err)
	}
}

// Protected buffer: Bytes is nil, Use exposes the secret, CopyOut works,
// and the region is re-protected afterwards.
func TestProtectedRoundTrip(t *testing.T) {
	b, err := NewProtected(48)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Bytes() != nil {
		t.Fatal("protected Bytes() must be nil")
	}
	if err := b.CopyFrom(bytes.Repeat([]byte{0x11}, 48)); err != nil {
		t.Fatal(err)
	}
	seen := false
	if err := b.Use(func(s []byte) {
		seen = true
		if !bytes.Equal(s, bytes.Repeat([]byte{0x11}, 48)) {
			t.Error("secret mismatch inside Use")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("Use did not run callback")
	}
	out := b.CopyOut()
	if !bytes.Equal(out, bytes.Repeat([]byte{0x11}, 48)) {
		t.Fatal("CopyOut mismatch")
	}
	WipeBytes(out)
}

// Finalizer safety net: an unclosed buffer is released by the GC.
func TestFinalizerReleases(t *testing.T) {
	before := PoolStats()
	func() {
		b, err := New(32)
		if err != nil {
			t.Fatal(err)
		}
		_ = b.CopyFrom(bytes.Repeat([]byte{9}, 32))
	}()
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	after := PoolStats()
	if after.CellsAlloc != before.CellsAlloc {
		t.Fatalf("finalizer did not release cell: alloc before=%d after=%d", before.CellsAlloc, after.CellsAlloc)
	}
}

// Concurrency under -race: parallel alloc/fill/use/close across classes.
func TestConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				b, err := New(32 + (i%4)*64)
				if err != nil {
					t.Error(err)
					return
				}
				n := b.Len()
				if err := b.CopyFrom(bytes.Repeat([]byte{seed}, n)); err != nil {
					t.Error(err)
					return
				}
				out := b.CopyOut()
				if !bytes.Equal(out, bytes.Repeat([]byte{seed}, n)) {
					t.Error("mismatch")
				}
				WipeBytes(out)
				if err := b.Close(); err != nil && err != ErrCanary {
					t.Error(err)
					return
				}
			}
		}(byte(g + 1))
	}
	wg.Wait()
}

// GC stress: allocation/free cycle under maximum GC pressure must not lose
// cells or leak arenas.
func TestGCStress(t *testing.T) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)
	for i := 0; i < 2000; i++ {
		b, err := New(96)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.CopyFrom(bytes.Repeat([]byte{byte(i)}, 96)); err != nil {
			t.Fatal(err)
		}
		if i%3 == 0 {
			_ = b.Close()
		}
		// every third buffer is left to the finalizer
	}
	runtime.GC()
	runtime.GC()
	s := PoolStats()
	if s.Arenas > 8 {
		t.Fatalf("arena count grew under GC stress: %d", s.Arenas)
	}
}

// 100k alloc/free cycles must not grow mappings or RSS without bound.
func TestAllocFreeCyclesNoLeak(t *testing.T) {
	if isHeapFallback {
		t.Skip("heap fallback has no mappings to leak")
	}
	const cycles = 100000
	b0 := PoolStats()
	rss0 := readRSS(t)
	for i := 0; i < cycles; i++ {
		b, err := New(80)
		if err != nil {
			t.Fatal(err)
		}
		if err := b.CopyFrom(bytes.Repeat([]byte{byte(i)}, 80)); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
	}
	b1 := PoolStats()
	rss1 := readRSS(t)
	if b1.MappedBytes > b0.MappedBytes+(4<<20) {
		t.Fatalf("mapped bytes grew: before=%d after=%d", b0.MappedBytes, b1.MappedBytes)
	}
	if rss1 > rss0+64*1024*1024 {
		t.Fatalf("RSS grew: before=%d after=%d", rss0, rss1)
	}
}
