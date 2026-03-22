// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package identity

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"testing"
)

func BenchmarkKnownDestinationsScale(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			// Clear map for each run
			knownDestinationsLock.Lock()
			knownDestinations = make(map[string][]any)
			knownDestinationsLock.Unlock()

			// Fill cache
			for range size {
				h := make([]byte, 16)
				_, _ = rand.Read(h)
				Remember([]byte("packet"), h, make([]byte, 64), []byte("appdata"))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				h := make([]byte, 16)
				// We use a small subset of the size for lookups to test hit performance
				for j := range 16 {
					h[j] = byte((i % size) >> (j * 8))
				}
				_, _ = Recall(h)
			}
		})
	}
}

func TestIdentityMemoryScale(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping identity memory test")
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	size := 100000
	t.Logf("Filling knownDestinations with %d entries...", size)

	for i := range size {
		h := make([]byte, 16)
		for j := range 16 {
			h[j] = byte(i >> (j * 8))
		}
		Remember([]byte("p"), h, make([]byte, 64), []byte("a"))
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	usedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory used for %d destinations: %.2f MB", size, usedMB)

	perEntry := (m2.Alloc - m1.Alloc) / uint64(size)
	t.Logf("Average per destination: %d bytes", perEntry)
}
