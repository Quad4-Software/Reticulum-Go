// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/announce"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/stamps"
)

type captureAnnounceHandler struct {
	last   []byte
	calls  int
	wantPR bool
}

func (h *captureAnnounceHandler) AspectFilter() []string     { return nil }
func (h *captureAnnounceHandler) ReceivePathResponses() bool { return h.wantPR }
func (h *captureAnnounceHandler) ReceivedAnnounce(_ []byte, _ any, appData []byte, _ uint8) error {
	h.calls++
	h.last = append([]byte(nil), appData...)
	return nil
}

// buildStampedAnnounceWire returns a signed announce packet whose app_data
// carries a valid 18-byte stamp ext at the given generation cost.
func buildStampedAnnounceWire(t *testing.T, appData []byte, genCost, lrCost float64, badStamp bool) (raw, destHash []byte, id *identity.Identity, nameHash []byte) {
	t.Helper()
	var err error
	id, err = identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destName := "stamp.test.dest"
	nh := sha256.Sum256([]byte(destName))
	nameHash = nh[:10]
	dh := sha256.Sum256(append(append([]byte{}, nameHash...), id.Hash()...))
	destHash = dh[:16]

	var stamp []byte
	material := append(append(append([]byte{}, destHash...), id.GetPublicKey()...), nameHash...)
	if badStamp {
		stamp = make([]byte, stamps.StampLength)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stamp, _, err = stamps.GenerateStamp(ctx, material, genCost, stamps.StampLength)
		if err != nil {
			t.Fatalf("GenerateStamp: %v", err)
		}
	}

	a, err := announce.New(id, destHash, destName, appData, false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatalf("announce.New: %v", err)
	}
	a.SetStamp(stamp, lrCost)
	raw, err = a.CreatePacket()
	if err != nil {
		t.Fatalf("CreatePacket: %v", err)
	}
	return raw, destHash, id, nameHash
}

func newStampTestTransport(t *testing.T, stampsCfg common.StampsConfig) (*Transport, *simIface, *captureAnnounceHandler) {
	t.Helper()
	cfg := &common.ReticulumConfig{EnableTransport: true, Stamps: stampsCfg}
	tr := NewTransport(cfg)
	tr.SetIdentity(mustStampIdentity(t))
	iface := newSimIface("stampin")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register: %v", err)
	}
	tr.ifaceStates.put(iface.GetName(), &ifaceState{})
	h := &captureAnnounceHandler{wantPR: true}
	tr.RegisterAnnounceHandler(h)
	t.Cleanup(func() {
		tr.Close()
		iface.stop()
	})
	return tr, iface, h
}

func TestStampedAnnounceStripsAndRecords(t *testing.T) {
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{DestinationCost: 8, DestinationCostSet: true})
	raw, destHash, _, _ := buildStampedAnnounceWire(t, []byte("hello-app"), 10, 12.5, false)
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d", h.calls)
	}
	if string(h.last) != "hello-app" {
		t.Fatalf("handler appData = %q, want stripped", h.last)
	}
	if v, ok := identity.RecallStampValue(destHash); !ok || v < 10 {
		t.Fatalf("RecallStampValue = %v,%v", v, ok)
	}
	if c, ok := identity.RecallLinkCost(destHash); !ok || c != 12.5 {
		t.Fatalf("RecallLinkCost = %v,%v", c, ok)
	}
}

func TestStampedAnnounceNoAppData(t *testing.T) {
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{DestinationCost: 8, DestinationCostSet: true})
	raw, _, _, _ := buildStampedAnnounceWire(t, nil, 10, 30.25, false)
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d", h.calls)
	}
	if len(h.last) != 0 {
		t.Fatalf("handler appData = %x, want empty", h.last)
	}
}

func TestInvalidStampKeepsAppData(t *testing.T) {
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{DestinationCost: 23.5, DestinationCostSet: true})
	// Generate at cost 8: below the required 23.5, so the ext format is
	// known but the stamp is invalid. app_data must stay untouched.
	raw, destHash, _, _ := buildStampedAnnounceWire(t, []byte("payload"), 8, 12.5, false)
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d", h.calls)
	}
	if len(h.last) != len("payload")+stamps.ExtPayloadLen {
		t.Fatalf("invalid stamp must not strip app_data, got %d bytes", len(h.last))
	}
	// Python still records stamp and value, but no lr_cost.
	if _, ok := identity.RecallStampValue(destHash); !ok {
		t.Fatal("stamp value not recorded")
	}
	if _, ok := identity.RecallLinkCost(destHash); ok {
		t.Fatal("invalid stamp must not record lr_cost")
	}
}

// buildRawAnnounceWire assembles a signed announce with an arbitrary
// app_data trailer (signed over, like a stamped ext with foreign flags).
func buildRawAnnounceWire(t *testing.T, appData []byte) []byte {
	t.Helper()
	id := mustStampIdentity(t)
	destName := "raw.test"
	nh := sha256.Sum256([]byte(destName))
	nameHash := nh[:10]
	dh := sha256.Sum256(append(append([]byte{}, nameHash...), id.Hash()...))
	destHash := dh[:16]
	randomHash := []byte("0123456789")

	signed := append([]byte{}, destHash...)
	signed = append(signed, id.GetPublicKey()...)
	signed = append(signed, nameHash...)
	signed = append(signed, randomHash...)
	signed = append(signed, appData...)
	sig, err := id.Sign(signed)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	pkt := []byte{0x01, 0x00} // header flags + hops
	pkt = append(pkt, destHash...)
	pkt = append(pkt, 0x00) // context
	pkt = append(pkt, id.GetPublicKey()...)
	pkt = append(pkt, nameHash...)
	pkt = append(pkt, randomHash...)
	pkt = append(pkt, sig...)
	pkt = append(pkt, appData...)
	return pkt
}

func TestUnknownExtFormatKeepsAppData(t *testing.T) {
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{DestinationCost: 8, DestinationCostSet: true})
	// Ext-shaped trailer with the anchor MSB (extended flag) set: unknown
	// format, app_data must not be stripped even though the signature is
	// valid over it.
	ext := []byte{stamps.ExtPayloadMarker, 0x00}
	ext = append(ext, make([]byte, stamps.StampLength)...)
	ext = append(ext, 0x00, 0x80)
	raw := buildRawAnnounceWire(t, append([]byte("payload"), ext...))
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler calls=%d", h.calls)
	}
	if len(h.last) != len("payload")+stamps.ExtPayloadLen {
		t.Fatalf("unknown-format ext must not strip, got %d", len(h.last))
	}
}

func TestStampsEnforcedDrops(t *testing.T) {
	tr, iface, h := newStampTestTransport(t, common.StampsConfig{
		EnforceStamps: true, DestinationCost: 10, DestinationCostSet: true,
	})
	// Unstamped announce (no ext) must be dropped.
	id := mustStampIdentity(t)
	destName := "plain.test"
	nh := sha256.Sum256([]byte(destName))
	dh := sha256.Sum256(append(append([]byte{}, nh[:10]...), id.Hash()...))
	a, err := announce.New(id, dh[:16], destName, []byte("x"), false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := a.CreatePacket()
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.handleAnnouncePacket(raw, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 0 {
		t.Fatal("unstamped announce reached handler under enforcement")
	}

	// Stamped but below the required cost must be dropped too.
	raw2, _, _, _ := buildStampedAnnounceWire(t, []byte("y"), 6, 12.5, false)
	if err := tr.handleAnnouncePacket(raw2, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 0 {
		t.Fatal("under-cost stamped announce reached handler under enforcement")
	}

	// A stamp meeting the required cost passes.
	raw3, _, _, _ := buildStampedAnnounceWire(t, []byte("z"), 12, 12.5, false)
	if err := tr.handleAnnouncePacket(raw3, iface); err != nil {
		t.Fatalf("handleAnnouncePacket: %v", err)
	}
	if h.calls != 1 {
		t.Fatal("valid stamped announce dropped under enforcement")
	}
}

func mustStampIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	return id
}
