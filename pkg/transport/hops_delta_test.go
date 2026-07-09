// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

func TestLocalHopsDeltaInit(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{LocalHopsDelta: true})
	if tr.localHopsDelta < 2 || tr.localHopsDelta > 7 {
		t.Fatalf("delta=%d want 2-7", tr.localHopsDelta)
	}
}

func TestShouldApplyLocalHopsDelta(t *testing.T) {
	tr := &Transport{
		config:         &common.ReticulumConfig{},
		localHopsDelta: 3,
	}
	p := &packet.Packet{Hops: 0, DestinationType: packet.DestinationSingle}
	if !tr.shouldApplyLocalHopsDelta(p, nil) {
		t.Fatal("expected apply for hop-0 single")
	}
	p.Hops = 1
	if tr.shouldApplyLocalHopsDelta(p, nil) {
		t.Fatal("should not apply when hops != 0")
	}
	p.Hops = 0
	p.DestinationType = packet.DestinationPlain
	if tr.shouldApplyLocalHopsDelta(p, nil) {
		t.Fatal("should not apply for plain destinations")
	}
	tr.config.ConnectedToSharedInstance = true
	p.DestinationType = packet.DestinationSingle
	if tr.shouldApplyLocalHopsDelta(p, nil) {
		t.Fatal("should not apply when connected to shared instance")
	}
}

func TestApplyLocalHopsDeltaIfNeeded(t *testing.T) {
	tr := &Transport{
		config:         &common.ReticulumConfig{},
		localHopsDelta: 5,
	}
	p := &packet.Packet{Hops: 0, DestinationType: packet.DestinationSingle, Packed: true}
	tr.applyLocalHopsDeltaIfNeeded(p, nil)
	if p.Hops != 5 {
		t.Fatalf("hops=%d want 5", p.Hops)
	}
	if p.Packed {
		t.Fatal("packet should be marked unpacked after hop rewrite")
	}
}
