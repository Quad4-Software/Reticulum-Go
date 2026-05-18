// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

func BenchmarkRoutingTableScale(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	sizes := []int{10, 100, 1000, 10000, 100000, 1000000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			tr := NewTransport(&common.ReticulumConfig{})
			defer tr.Close()

			// Mock interface
			iface := &mockInterface{}
			iface.Name = "eth0"
			_ = tr.RegisterInterface("eth0", iface)

			// Pre-fill routing table
			hashes := make([][]byte, size)
			for i := range size {
				h := make([]byte, 16)
				_, _ = rand.Read(h)
				hashes[i] = h
				tr.UpdatePath(h, nil, "eth0", uint8(i%10))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Pick a random hash from the table
				idx := i % size
				_ = tr.NextHop(hashes[idx])
				_ = tr.NextHopInterface(hashes[idx])
				_ = tr.HopsTo(hashes[idx])
			}
		})
	}
}

func BenchmarkRoutingTableUpdates(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	sizes := []int{10, 100, 1000, 10000, 100000, 1000000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			tr := NewTransport(&common.ReticulumConfig{})
			defer tr.Close()

			iface := &mockInterface{}
			iface.Name = "eth0"
			_ = tr.RegisterInterface("eth0", iface)

			hashes := make([][]byte, b.N)
			for i := 0; i < b.N; i++ {
				h := make([]byte, 16)
				_, _ = rand.Read(h)
				hashes[i] = h
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				tr.UpdatePath(hashes[i], nil, "eth0", uint8(i%10))
			}
		})
	}
}
