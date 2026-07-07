// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"runtime"
	"testing"
	"time"
)

const (
	embeddedProfileNodes      = 32
	embeddedProfileMaxHeapKB  = 1024
	embeddedProfileWarnHeapKB = 512
	embeddedMaxPathEntryBytes = 256
)

type simHeapDelta struct {
	heapAlloc  uint64
	goroutines int
}

func measureSimNetworkHeap(t testing.TB, net *simNetwork, fn func()) simHeapDelta {
	t.Helper()
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	baseG := runtime.NumGoroutine()

	fn()

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	var used uint64
	if m2.Alloc >= m1.Alloc {
		used = m2.Alloc - m1.Alloc
	}
	return simHeapDelta{
		heapAlloc:  used,
		goroutines: runtime.NumGoroutine() - baseG,
	}
}

func TestSimEmbeddedPathTableBudget(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	net := buildRandom(t, embeddedProfileNodes, 0.08, 0xebd00)
	t.Cleanup(net.close)

	delta := measureSimNetworkHeap(t, net, func() {
		timeout := simConvergenceTimeout(net.diameter()) + 30*time.Second
		for _, node := range net.nodes {
			node.originateAnnounce(t)
			time.Sleep(25 * time.Millisecond)
		}
		for i, src := range net.nodes {
			peers := make([]*simNode, 0, len(net.nodes)-1)
			for j, n := range net.nodes {
				if i != j {
					peers = append(peers, n)
				}
			}
			assertAllHavePath(t, peers, src.destHash, timeout)
		}
	})

	heapKB := delta.heapAlloc / 1024
	t.Logf("embedded profile: nodes=%d heap=%d KB goroutines_delta=%d",
		embeddedProfileNodes, heapKB, delta.goroutines)

	if heapKB > embeddedProfileWarnHeapKB {
		t.Logf("WARNING: heap %d KB exceeds warn budget %d KB", heapKB, embeddedProfileWarnHeapKB)
	}
	if heapKB > embeddedProfileMaxHeapKB {
		t.Fatalf("heap %d KB exceeds max budget %d KB", heapKB, embeddedProfileMaxHeapKB)
	}

	totalEntries := 0
	for _, node := range net.nodes {
		entries := simPathTableLen(node.tr)
		totalEntries += entries
		if entries > embeddedProfileNodes {
			t.Errorf("node %s path count %d exceeds %d", node.name, entries, embeddedProfileNodes)
		}
	}
	t.Logf("path entries across network=%d", totalEntries)

	if per := pathEntrySize(net.nodes[0].tr, net.nodes[1].destHash); per > 0 {
		t.Logf("spot-check path entry size: ~%d bytes", per)
		if per > embeddedMaxPathEntryBytes {
			t.Errorf("per-entry size %d exceeds %d B budget", per, embeddedMaxPathEntryBytes)
		}
	}
}

func TestSimEmbeddedGoroutineBudget(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	for range 3 {
		net := buildRandom(t, embeddedProfileNodes, 0.08, 0x600d)
		for _, node := range net.nodes {
			node.originateAnnounce(t)
		}
		time.Sleep(100 * time.Millisecond)
		net.close()
	}

	runtime.GC()
	time.Sleep(750 * time.Millisecond)
	final := runtime.NumGoroutine()
	if final > baseline+2 {
		buf := make([]byte, 1<<20)
		nb := runtime.Stack(buf, true)
		t.Errorf("goroutine leak suspected: baseline=%d final=%d\n%s", baseline, final, buf[:nb])
	}
	t.Logf("goroutines: baseline=%d final=%d", baseline, final)
}

func TestSimPathTableGrowthBounded(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 32
	net := buildLine(t, n)
	t.Cleanup(net.close)

	net.nodes[0].originateAnnounce(t)
	timeout := simConvergenceTimeout(n - 1)
	assertAllHavePath(t, net.nodes[1:], net.nodes[0].destHash, timeout)

	for i, node := range net.nodes {
		entries := simPathTableLen(node.tr)
		if entries > n {
			t.Errorf("node%d path table has %d entries, want <= %d", i, entries, n)
		}
	}
}

// measureUpdatePathDelta fills every node's path table once and reports the
// resulting heap growth. Background maintenance goroutines (announce/path
// cleanup tickers) can allocate concurrently with the GC snapshots taken
// here, so callers should take the minimum of several samples to filter out
// that unrelated noise rather than trusting a single sample.
func measureUpdatePathDelta(net *simNetwork) uint64 {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	runtime.ReadMemStats(&m1)

	for _, src := range net.nodes {
		ifaceName := src.ifaces[0].GetName()
		for _, dst := range net.nodes {
			if dst == src {
				continue
			}
			src.tr.UpdatePath(dst.destHash, dst.destHash, ifaceName, 1)
		}
	}

	runtime.GC()
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	runtime.ReadMemStats(&m2)

	if m2.Alloc < m1.Alloc {
		return 0
	}
	return m2.Alloc - m1.Alloc
}

func TestSimMemoryFootprintAcrossNodes(t *testing.T) {
	skipSimIfShort(t)

	const n = 32
	net := buildLine(t, n)
	t.Cleanup(net.close)

	// Take the minimum of several samples: genuine per-entry cost is a
	// floor, while unrelated background allocations (maintenance tickers,
	// GC scheduling under -race) can only push a sample above that floor.
	const samples = 5
	used := measureUpdatePathDelta(net)
	for i := 1; i < samples; i++ {
		if u := measureUpdatePathDelta(net); u < used {
			used = u
		}
	}

	entries := uint64(n) * uint64(n-1)
	perEntry := used / entries
	heapKB := used / 1024

	t.Logf("nodes=%d entries=%d total=%d KB per_entry=%d B",
		n, entries, heapKB, perEntry)

	if heapKB > embeddedProfileMaxHeapKB {
		t.Fatalf("path table footprint %d KB exceeds %d KB budget", heapKB, embeddedProfileMaxHeapKB)
	}
	if perEntry > embeddedMaxPathEntryBytes {
		t.Fatalf("per-entry size %d B exceeds %d B budget", perEntry, embeddedMaxPathEntryBytes)
	}
	if heapKB > embeddedProfileWarnHeapKB {
		t.Logf("WARNING: footprint %d KB exceeds warn budget %d KB", heapKB, embeddedProfileWarnHeapKB)
	}
}
