// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common_test

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
)

func TestApplyIFACInboundCountsFailuresAndOK(t *testing.T) {
	health.Default.Reset()
	iface := &common.BaseInterface{Name: "health-ifac-test"}

	// No IFAC configured: flagged packet must drop and count ifac_fail.
	flagged := []byte{0x80, 0x01, 0x02}
	if _, ok := common.ApplyIFACInbound(iface, flagged); ok {
		t.Fatal("expected drop for IFAC-flagged packet without IFAC configured")
	}
	snap := health.Default.SnapshotIface("health-ifac-test")
	if snap.IFACFail.Total != 1 {
		t.Fatalf("ifac_fail=%d want 1", snap.IFACFail.Total)
	}

	// Clean packet without IFAC continues and counts rx_ok.
	clean := []byte{0x00, 0x01, 0x02}
	out, ok := common.ApplyIFACInbound(iface, clean)
	if !ok {
		t.Fatal("expected accept for unflagged packet")
	}
	if len(out) != len(clean) {
		t.Fatalf("out len=%d want %d", len(out), len(clean))
	}
	snap = health.Default.SnapshotIface("health-ifac-test")
	if snap.RxOK.Total != 1 {
		t.Fatalf("rx_ok=%d want 1", snap.RxOK.Total)
	}
	if snap.IFACFail.Total != 1 {
		t.Fatalf("ifac_fail should stay 1, got %d", snap.IFACFail.Total)
	}
}

func BenchmarkApplyIFACInboundAccepted(b *testing.B) {
	health.Default.Reset()
	iface := &common.BaseInterface{Name: "bench-ifac"}
	// Warm iface slot so Inc is alloc-free.
	health.Inc("bench-ifac", health.KindRxOK)
	pkt := []byte{0x00, 0x01, 0x02, 0x03}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := common.ApplyIFACInbound(iface, pkt)
		if !ok {
			b.Fatal("expected accept")
		}
	}
}
