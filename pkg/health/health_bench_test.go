// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import (
	"testing"
)

func BenchmarkIncExistingIface(b *testing.B) {
	r := NewRegistry()
	r.Inc("udp0", KindRxOK)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Inc("udp0", KindRxOK)
	}
}

func BenchmarkIncTransportOnly(b *testing.B) {
	r := NewRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Inc("", KindRxOK)
	}
}

func BenchmarkIncParallel(b *testing.B) {
	r := NewRegistry()
	r.Inc("udp0", KindRxOK)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Inc("udp0", KindRxOK)
		}
	})
}

func BenchmarkSnapshotIface(b *testing.B) {
	r := NewRegistry()
	for range 100 {
		r.Inc("udp0", KindRxOK)
		r.Inc("udp0", KindIFACFail)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.SnapshotIface("udp0")
	}
}

func BenchmarkWindowedAdd(b *testing.B) {
	var w windowedCounter
	now := int64(1_700_000_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.add(1, now)
	}
}
