// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import (
	"testing"
)

// FuzzIncAndSnapshot never panics on arbitrary iface names and kinds.
func FuzzIncAndSnapshot(f *testing.F) {
	f.Add("udp0", uint8(0))
	f.Add("", uint8(1))
	f.Add("lo", uint8(255))
	f.Add(string(make([]byte, 256)), uint8(13))

	f.Fuzz(func(t *testing.T, iface string, kindByte uint8) {
		if len(iface) > 4096 {
			t.Skip()
		}
		r := NewRegistry()
		r.Inc(iface, Kind(kindByte))
		_ = r.SnapshotTransport()
		_ = r.SnapshotIface(iface)
		r.Inc(iface, KindRxOK)
		_ = r.SnapshotIface(iface)
	})
}

// FuzzWindowedCounter exercises bucket advance across arbitrary timelines.
func FuzzWindowedCounter(f *testing.F) {
	f.Add(int64(0), uint64(1), int64(0))
	f.Add(int64(1_700_000_000), uint64(5), int64(70))
	f.Add(int64(-100), uint64(3), int64(1000))

	f.Fuzz(func(t *testing.T, base int64, n uint64, delta int64) {
		if n > 1<<20 {
			n = 1 << 20
		}
		var w windowedCounter
		w.add(n, base)
		later := base + delta
		w.add(1, later)
		total, r60, r300 := w.snapshot(later)
		if total < 1 {
			t.Fatalf("total=%d", total)
		}
		if r60 > total || r300 > total {
			t.Fatalf("rates exceed total: 60=%d 300=%d total=%d", r60, r300, total)
		}
	})
}
