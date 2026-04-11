// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package transport

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
)

func BenchmarkSeenAnnouncesScale(b *testing.B) {
	sizes := []int{1000, 10000, 100000, 1000000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			tr := NewTransport(&common.ReticulumConfig{})
			defer tr.Close()

			// Fill seenAnnounces
			for range size {
				h := make([]byte, 32)
				_, _ = rand.Read(h)
				tr.seenAnnounces[string(h)] = time.Now()
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Use a predictable but changing string
				tr.mutex.Lock()
				_ = tr.seenAnnounces[fmt.Sprint(i%size)]
				tr.mutex.Unlock()
			}
		})
	}
}

func BenchmarkReceiptRegistryScale(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			tr := NewTransport(&common.ReticulumConfig{})
			defer tr.Close()

			p := &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeData,
				DestinationHash: make([]byte, 16),
				Data:            []byte("benchmark"),
			}
			if err := p.Pack(); err != nil {
				b.Fatalf("Failed to pack packet: %v", err)
			}

			receipts := make([]*packet.PacketReceipt, size)
			for i := range size {
				receipts[i] = packet.NewPacketReceipt(p)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				tr.RegisterReceipt(receipts[i%size])
				if i%100 == 0 {
					tr.cleanupExpiredReceipts()
				}
			}
		})
	}
}

func TestTransportMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	// Register interface to avoid "Interface not found" logs and early return
	iface := &mockInterface{}
	iface.Name = "eth0"
	_ = tr.RegisterInterface("eth0", iface)

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	size := 1000000
	t.Logf("Filling transport with %d routes...", size)

	for i := range size {
		h := make([]byte, 16)
		// Optimization: Use a simple loop for hash generation to avoid crypto overhead
		for j := range 16 {
			h[j] = byte(i >> (j * 8))
		}
		tr.UpdatePath(h, h, "eth0", uint8(i%255))
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	usedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory used for %d routes: %.2f MB", size, usedMB)
	t.Logf("Average per route: %d bytes", (m2.Alloc-m1.Alloc)/uint64(size))

	if usedMB > 500 {
		t.Errorf("Memory usage too high: %.2f MB", usedMB)
	}
}
