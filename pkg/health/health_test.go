// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import (
	"sync"
	"testing"
	"time"
)

func TestWindowedCounterRates(t *testing.T) {
	var w windowedCounter
	now := int64(1_700_000_000)
	for range 10 {
		w.add(1, now)
	}
	total, r60, r300 := w.snapshot(now)
	if total != 10 {
		t.Fatalf("total=%d want 10", total)
	}
	if r60 != 10 || r300 != 10 {
		t.Fatalf("rates 60=%d 300=%d want 10", r60, r300)
	}

	later := now + 70
	w.add(5, later)
	total, r60, r300 = w.snapshot(later)
	if total != 15 {
		t.Fatalf("total after later=%d want 15", total)
	}
	if r60 != 5 {
		t.Fatalf("rate60 after 70s=%d want 5 (old bucket aged out)", r60)
	}
	if r300 != 15 {
		t.Fatalf("rate300=%d want 15", r300)
	}
}

func TestWindowedCounterLargeGapClears(t *testing.T) {
	var w windowedCounter
	now := int64(1_700_000_000)
	w.add(7, now)
	far := now + int64(bucketCount+1)*bucketSpanSec
	w.add(1, far)
	_, r60, r300 := w.snapshot(far)
	if r60 != 1 || r300 != 1 {
		t.Fatalf("after gap rates 60=%d 300=%d want 1", r60, r300)
	}
	if total, _, _ := w.snapshot(far); total != 8 {
		t.Fatalf("total=%d want 8", total)
	}
}

func TestBucketSlotNonNegative(t *testing.T) {
	for _, idx := range []int64{-1, -60, -61, 0, 1, 59, 60, 121} {
		slot := bucketSlot(idx)
		if slot < 0 || slot >= bucketCount {
			t.Fatalf("bucketSlot(%d)=%d out of range", idx, slot)
		}
	}
}

func TestRegistryIncAndSnapshot(t *testing.T) {
	r := NewRegistry()
	r.Inc("udp0", KindIFACFail)
	r.Inc("udp0", KindIFACFail)
	r.Inc("udp0", KindRxOK)
	r.Inc("", KindHMACFail)

	iface := r.SnapshotIface("udp0")
	if iface.IFACFail.Total != 2 {
		t.Fatalf("ifac_fail=%d want 2", iface.IFACFail.Total)
	}
	if iface.RxOK.Total != 1 {
		t.Fatalf("rx_ok=%d want 1", iface.RxOK.Total)
	}
	if iface.IntegrityFailRate <= 0 {
		t.Fatalf("integrity_fail_rate=%v want >0", iface.IntegrityFailRate)
	}

	tr := r.SnapshotTransport()
	if tr.IFACFail.Total != 2 {
		t.Fatalf("transport ifac_fail=%d want 2", tr.IFACFail.Total)
	}
	if tr.HMACFail.Total != 1 {
		t.Fatalf("transport hmac_fail=%d want 1", tr.HMACFail.Total)
	}
	if r.SnapshotIface("missing").IFACFail.Total != 0 {
		t.Fatal("missing iface should be empty")
	}
}

func TestRegistryIncInvalidKind(t *testing.T) {
	r := NewRegistry()
	r.Inc("x", Kind(255))
	if r.SnapshotTransport().RxOK.Total != 0 {
		t.Fatal("invalid kind must be ignored")
	}
}

func TestRegistryConcurrentInc(t *testing.T) {
	r := NewRegistry()
	const goroutines = 8
	const per = 1000
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range per {
				r.Inc("udp0", KindRxOK)
				r.Inc("udp1", KindIFACFail)
			}
		}()
	}
	wg.Wait()
	want := uint64(goroutines * per)
	if got := r.SnapshotIface("udp0").RxOK.Total; got != want {
		t.Fatalf("udp0 rx_ok=%d want %d", got, want)
	}
	if got := r.SnapshotIface("udp1").IFACFail.Total; got != want {
		t.Fatalf("udp1 ifac_fail=%d want %d", got, want)
	}
	if got := r.SnapshotTransport().RxOK.Total; got != want {
		t.Fatalf("transport rx_ok=%d want %d", got, want)
	}
	if r.IfaceCount() != 2 {
		t.Fatalf("iface slots=%d want 2", r.IfaceCount())
	}
}

func TestRegistryReset(t *testing.T) {
	r := NewRegistry()
	r.Inc("a", KindHMACFail)
	r.Reset()
	if r.SnapshotIface("a").HMACFail.Total != 0 {
		t.Fatal("reset should clear iface counters")
	}
	if r.IfaceCount() != 0 {
		t.Fatal("reset should drop iface slots")
	}
}

func TestKindString(t *testing.T) {
	for k := range kindCount {
		if k.String() == "unknown" {
			t.Fatalf("kind %d should have a name", k)
		}
	}
	if Kind(255).String() != "unknown" {
		t.Fatal("out of range kind should be unknown")
	}
}

func TestNilRegistrySafe(t *testing.T) {
	var r *Registry
	r.Inc("x", KindRxOK)
	_ = r.SnapshotTransport()
	_ = r.SnapshotIface("x")
	r.Reset()
	if r.IfaceCount() != 0 {
		t.Fatal("nil IfaceCount")
	}
}

func TestWindowAdvanceDoesNotRaceWithSnapshot(t *testing.T) {
	var w windowedCounter
	base := time.Now().Unix()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 5000 {
			w.add(1, base+int64(i%200))
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 5000 {
			_, _, _ = w.snapshot(base + int64(i%200))
		}
	}()
	wg.Wait()
}
