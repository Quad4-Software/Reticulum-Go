// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/rate"
)

// announceReasonHarness builds a transport + sim iface for reject-reason
// counter tests.
func announceReasonHarness(t *testing.T) (*Transport, *simIface) {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	tr.SetIdentity(id)
	iface := newSimIface("reasons")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	tr.ifaceStates.put(iface.GetName(), &ifaceState{})
	t.Cleanup(func() {
		tr.Close()
		iface.stop()
	})
	return tr, iface
}

func TestAnnounceRejectMalformedShort(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	_ = tr.handleAnnouncePacket([]byte{0x00}, iface)
	ifaceSnap := health.Default.SnapshotIface(iface.GetName())
	if ifaceSnap.AnnounceMalformed.Total == 0 {
		t.Fatal("per-iface announce_malformed not incremented")
	}
	trSnap := health.Default.SnapshotTransport()
	if trSnap.AnnounceMalformed.Total == 0 {
		t.Fatal("transport announce_malformed not incremented")
	}
}

func TestAnnounceRejectDestType(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	pkt, _, _ := buildValidAnnounceForReceiver(t)
	pkt[0] = (pkt[0] &^ HeaderDestTypeMask) | (DestTypePlain << HeaderDestTypeShift)
	_ = tr.handleAnnouncePacket(pkt, iface)
	ifaceSnap := health.Default.SnapshotIface(iface.GetName())
	if ifaceSnap.AnnounceDestType.Total == 0 {
		t.Fatal("announce_dest_type not incremented for PLAIN announce")
	}
}

func TestAnnounceRejectMaxHops(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	pkt, _, _ := buildValidAnnounceForReceiver(t)
	pkt[1] = MaxHops
	_ = tr.handleAnnouncePacket(pkt, iface)
	ifaceSnap := health.Default.SnapshotIface(iface.GetName())
	if ifaceSnap.AnnounceMaxHops.Total == 0 {
		t.Fatal("announce_max_hops not incremented")
	}
}

func TestAnnounceRejectBlackholed(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	pkt, _, _ := buildValidAnnounceForReceiver(t)

	// Extract the announcing identity from the announce payload:
	// layout is header(2)+desthash(16)+context(1)+pubkey(64)+...
	pub := pkt[HeaderSize+AddrHashSize+ContextByteLen : HeaderSize+AddrHashSize+ContextByteLen+64]
	id := identity.FromPublicKey(pub)
	if id == nil {
		t.Fatal("could not derive announcing identity")
	}
	if _, err := tr.BlackholeTable().Add(id.Hash(), 0, "test"); err != nil {
		t.Fatalf("blackhole add: %v", err)
	}
	_ = tr.handleAnnouncePacket(pkt, iface)
	snap := health.Default.SnapshotIface(iface.GetName())
	if snap.AnnounceBlackholed.Total == 0 {
		t.Fatal("announce_blackholed not incremented")
	}
}

func TestAnnounceRejectSuppressedByRateLimit(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	// Zero-capacity limiter: every Allow() fails.
	tr.announceRate = rate.NewLimiter(0.0000001, 0)
	pkt, _, _ := buildValidAnnounceForReceiver(t)
	_ = tr.handleAnnouncePacket(pkt, iface)
	snap := health.Default.SnapshotIface(iface.GetName())
	if snap.AnnounceSuppressed.Total == 0 {
		t.Fatal("announce_suppressed not incremented under rate limit")
	}
}

func TestAnnounceRejectHeldByIngress(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	var pkt []byte
	st := tr.ifaceStates.get(iface.GetName())
	if st == nil {
		t.Fatal("missing iface state")
	}
	// Near-zero burst thresholds put the interface into burst mode after the
	// minimum sample count (8 arrivals). New-destination announces during a
	// burst are held by ingress control.
	icCfg := rate.NewIngressControlConfig()
	icCfg.NewTime = time.Hour
	icCfg.BurstFreqNew = 0.000001
	icCfg.BurstFreq = 0.000001
	st.ingress = rate.NewIngressControlWith(icCfg)
	for i := 0; i < 12; i++ {
		pkt, _, _ = buildValidAnnounceForReceiver(t)
		_ = tr.handleAnnouncePacket(pkt, iface)
	}
	snap := health.Default.SnapshotIface(iface.GetName())
	if snap.AnnounceHeld.Total == 0 {
		t.Fatal("announce_held not incremented")
	}
}

func TestAnnounceDupCounted(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	pkt, _, _ := buildValidAnnounceForReceiver(t)
	_ = tr.handleAnnouncePacket(pkt, iface)
	_ = tr.handleAnnouncePacket(pkt, iface)
	snap := health.Default.SnapshotIface(iface.GetName())
	if snap.AnnounceDup.Total == 0 {
		t.Fatal("announce_dup not incremented on replay")
	}
	if snap.AnnounceOK.Total == 0 {
		t.Fatal("announce_ok missing for first accepted announce")
	}
}

func TestInterfaceStatCarriesAnnounceReasons(t *testing.T) {
	tr, iface := announceReasonHarness(t)
	_ = tr.handleAnnouncePacket([]byte{0x00}, iface)
	stats := tr.GetInterfaceStatsRPC()
	var found *InterfaceStat
	for i := range stats.Interfaces {
		if stats.Interfaces[i].Name == iface.GetName() {
			found = &stats.Interfaces[i]
		}
	}
	if found == nil {
		t.Fatal("iface not in stats")
	}
	if found.AnnounceMalformed == 0 {
		t.Fatal("AnnounceMalformed not surfaced in interface_stats")
	}
}
