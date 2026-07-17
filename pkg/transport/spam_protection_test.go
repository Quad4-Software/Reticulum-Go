// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

func sentCount(ti *trackingIface) int {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	return len(ti.sent)
}

// signedAnnounce builds a valid announce. Dest expand name must be
// "reticulum-go.node" to match NewAnnouncePacket name hashing.
func signedAnnounce(t *testing.T, tr *Transport, id *identity.Identity) (raw, destHash []byte) {
	t.Helper()
	dest, err := destination.New(id, destination.In, destination.Single, "reticulum-go", tr, "node")
	if err != nil {
		t.Fatalf("destination.New: %v", err)
	}
	transportID := make([]byte, 16)
	pkt, err := packet.NewAnnouncePacket(dest.GetHash(), id, []byte("ad"), transportID)
	if err != nil {
		t.Fatalf("NewAnnouncePacket: %v", err)
	}
	raw, err = pkt.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	raw[1] = 0
	return raw, dest.GetHash()
}

func transportWithIfaceConfig(t *testing.T, inCfg, outCfg *common.InterfaceConfig) (tr *Transport, in, out *trackingIface) {
	t.Helper()
	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		Interfaces:      map[string]*common.InterfaceConfig{},
	}
	if inCfg != nil {
		cfg.Interfaces["in"] = inCfg
	}
	if outCfg != nil {
		cfg.Interfaces["out"] = outCfg
	}
	tr = NewTransport(cfg)

	in = newTrackingIface("in")
	out = newTrackingIface("out")
	if err := tr.RegisterInterface("in", in); err != nil {
		t.Fatalf("register in: %v", err)
	}
	if err := tr.RegisterInterface("out", out); err != nil {
		t.Fatalf("register out: %v", err)
	}
	return tr, in, out
}

func TestIngressControlHoldsAnnounceFlood(t *testing.T) {
	inCfg := &common.InterfaceConfig{
		IngressControlSet:     true,
		IngressControl:        true,
		ICBurstFreq:           4,
		ICBurstFreqNew:        4,
		ICNewTime:             1,
		ICBurstHold:           5,
		ICBurstPenalty:        5,
		ICMaxHeldAnnounces:    16,
		ICHeldReleaseInterval: 1,
	}
	tr, _, out := transportWithIfaceConfig(t, inCfg, nil)
	defer tr.Close()

	in, _ := tr.GetInterface("in")
	if in == nil {
		t.Fatal("missing in iface")
	}

	for range 12 {
		id, err := identity.New()
		if err != nil {
			t.Fatalf("identity.New: %v", err)
		}
		raw, _ := signedAnnounce(t, tr, id)
		tr.HandlePacket(raw, in)
	}
	time.Sleep(150 * time.Millisecond)

	st := tr.ifaceStates.get("in")
	if st == nil || st.ingress == nil {
		t.Fatal("expected ingress state on in iface")
	}
	if !st.ingress.InBurst() && st.ingress.HeldCount() == 0 {
		t.Fatal("ingress control did not engage under flood")
	}
	if got := sentCount(out); got >= 12 {
		t.Fatalf("expected forward fan-out to be suppressed by ingress control; sent=%d", got)
	}
}

func TestIngressControlOffByConfig(t *testing.T) {
	inCfg := &common.InterfaceConfig{
		IngressControlSet: true,
		IngressControl:    false,
	}
	tr, _, _ := transportWithIfaceConfig(t, inCfg, nil)
	defer tr.Close()

	st := tr.ifaceStates.get("in")
	if st == nil || st.ingress == nil {
		t.Fatal("expected ingress state on in iface")
	}
	if st.ingress.Enabled() {
		t.Fatal("ingress should be disabled by config")
	}
}

func TestEgressAnnounceRateControlSuppressesRebroadcast(t *testing.T) {
	outCfg := &common.InterfaceConfig{
		AnnounceRateTarget:  60.0,
		AnnounceRateGrace:   1,
		AnnounceRatePenalty: 0,
	}
	tr, _, out := transportWithIfaceConfig(t, nil, outCfg)
	defer tr.Close()

	in, _ := tr.GetInterface("in")
	if in == nil {
		t.Fatal("in iface missing")
	}

	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}

	for range 3 {
		raw, _ := signedAnnounce(t, tr, id)
		tr.HandlePacket(raw, in)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(800 * time.Millisecond)

	if got := sentCount(out); got > 2 {
		t.Fatalf("egress AnnounceRateControl did not suppress rebroadcast; out sent=%d", got)
	}
}

func TestRegisterInterfaceMatchesConfigByName(t *testing.T) {
	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		Interfaces: map[string]*common.InterfaceConfig{
			"section_key_unrelated": {
				Name:               "wire-name",
				AnnounceRateTarget: 10,
			},
		},
	}
	tr := NewTransport(cfg)
	defer tr.Close()

	iface := newTrackingIface("wire-name")
	if err := tr.RegisterInterface("wire-name", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	st := tr.ifaceStates.get("wire-name")
	if st == nil || st.egress == nil {
		t.Fatal("expected egress AnnounceRateControl resolved by name match")
	}
}

func TestReleaseHeldAnnouncesIsNoOpWhenEmpty(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	iface := newTrackingIface("a")
	if err := tr.RegisterInterface("a", iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	tr.releaseHeldAnnounces()
	if got := sentCount(iface); got != 0 {
		t.Fatalf("nothing should have been emitted; sent=%d", got)
	}
}

func TestBuildIfaceStateDefaults(t *testing.T) {
	st := buildIfaceState(nil)
	if st.ingress == nil || !st.ingress.Enabled() {
		t.Fatal("default ingress should be enabled")
	}
	if st.egress != nil {
		t.Fatal("egress AnnounceRateControl must be nil unless opted in")
	}
	if st.announceCap != 2.0 {
		t.Fatalf("default announce_cap = %v, want 2.0", st.announceCap)
	}
}
