// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// TestTransportSoakFaultLoad runs a bounded mesh under combined loss, corrupt,
// and flap load, then asserts heap and goroutine deltas stay within budget.
func TestTransportSoakFaultLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak in -short mode")
	}
	secs := 90
	if v := os.Getenv("SOAK_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 5 {
			t.Fatalf("SOAK_SECONDS=%q invalid", v)
		}
		secs = n
	}
	duration := time.Duration(secs) * time.Second

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	baseG := runtime.NumGoroutine()

	const n = 6
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)

	net.link(t, 0, 1)
	leftLoss, _ := linkLossy(t, net, 1, 2, 0.12, 0.12, 0x50a1)
	leftCorrupt, _ := linkCorrupt(t, net, 2, 3, corruptBitFlip, 0.2, 0x50a2)
	leftFlap, rightFlap := linkFlap(t, net, 3, 4)
	net.link(t, 4, 5)

	src := net.nodes[0]
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		flap := false
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				src.originateAnnounce(t)
				flap = !flap
				leftFlap.setDown(flap)
				rightFlap.setDown(flap)
			}
		}
	}()

	time.Sleep(duration)
	close(stop)
	<-done
	leftFlap.setDown(false)
	rightFlap.setDown(false)

	took, ok := waitForPaths(net.nodes[1:], src.destHash, 8*time.Second)
	t.Logf("soak: %d/%d paths after settle %v drops=%d muts=%d",
		ok, n-1, took, atomic.LoadUint64(&leftLoss.dropped), atomic.LoadUint64(&leftCorrupt.mutated))
	if ok == 0 {
		t.Fatal("soak: zero paths after fault load")
	}

	net.close()
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	finalG := runtime.NumGoroutine()

	heapDelta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	const maxHeapDelta = 64 << 20
	if heapDelta > maxHeapDelta {
		t.Fatalf("soak heap delta %d exceeds %d", heapDelta, maxHeapDelta)
	}
	const maxGoroutineDelta = 24
	if finalG > baseG+maxGoroutineDelta {
		t.Fatalf("soak goroutine delta baseline=%d final=%d", baseG, finalG)
	}
	t.Logf("soak ok: heap_delta=%d goroutines=%d->%d", heapDelta, baseG, finalG)
}

func TestPacketHashlistAtCapNoGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hashlist cap soak in -short mode")
	}
	hl := newPacketHashList(10_000)
	h := make([]byte, 32)
	fill := func(off int, n int) {
		for i := range n {
			binary.LittleEndian.PutUint32(h, uint32(off+i))
			hl.add(h)
			_ = hl.seen(h)
		}
	}
	fill(0, 30_000)
	runtime.GC()
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	fill(30_000, 30_000)
	runtime.GC()
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	if m2.HeapAlloc > m1.HeapAlloc+(4<<20) {
		t.Fatalf("hashlist at cap grew %d -> %d", m1.HeapAlloc, m2.HeapAlloc)
	}
	if hl.Len() > 10_000+5_000 {
		t.Fatalf("len=%d exceeds rotate bound", hl.Len())
	}
}
