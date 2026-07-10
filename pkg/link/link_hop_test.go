// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func TestValidateLinkProof_HopMismatch(t *testing.T) {
	ident, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	dest, err := destination.New(ident, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}

	iface := newNoopIface("wan")
	l := NewLink(dest, tr, iface, nil, nil)
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.linkID = bytes.Repeat([]byte{0x11}, 16)
	l.initiator = true
	l.expectedHops = 3
	l.status.Store(int32(StatusPending))

	proof := &packet.Packet{
		PacketType:      packet.PacketTypeProof,
		Context:         packet.ContextLRProof,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Hops:            5, // wire 5 → accounted 6 on wan
		Data:            bytes.Repeat([]byte{0xAB}, identity.SigLength/8+KeySize),
	}
	err = l.ValidateLinkProof(proof, iface)
	if err == nil {
		t.Fatal("expected hop mismatch error")
	}
	if !strings.Contains(err.Error(), "hop count mismatch") {
		t.Fatalf("want hop mismatch, got %v", err)
	}
}

func TestValidateLinkProof_AccountedHopsMatch(t *testing.T) {
	ident, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	dest, err := destination.New(ident, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}
	iface := newNoopIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}
	tr.UpdatePath(dest.GetHash(), bytes.Repeat([]byte{0x77}, 16), "wan", 1)

	l := NewLink(dest, tr, iface, nil, nil)
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.linkID = bytes.Repeat([]byte{0x13}, 16)
	l.initiator = true
	l.expectedHops = 1
	l.status.Store(int32(StatusPending))

	proof := &packet.Packet{
		PacketType:      packet.PacketTypeProof,
		Context:         packet.ContextLRProof,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Hops:            0, // wire 0 → accounted 1
		Data:            bytes.Repeat([]byte{0xCD}, identity.SigLength/8+KeySize),
	}
	err = l.ValidateLinkProof(proof, iface)
	if err == nil {
		t.Fatal("expected signature failure after hop gate passes")
	}
	if strings.Contains(err.Error(), "hop count mismatch") {
		t.Fatalf("accounted hops should match expectedHops=1, got %v", err)
	}
}

func TestValidateLinkProof_UnknownHopsAllowsAny(t *testing.T) {
	ident, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	dest, err := destination.New(ident, destination.Out, destination.Single, "app", tr, "peer")
	if err != nil {
		t.Fatal(err)
	}

	l := NewLink(dest, tr, nil, nil, nil)
	if err := l.generateEphemeralKeys(); err != nil {
		t.Fatal(err)
	}
	l.linkID = bytes.Repeat([]byte{0x12}, 16)
	l.initiator = true
	l.expectedHops = transport.PathfinderM
	l.status.Store(int32(StatusPending))

	proof := &packet.Packet{
		PacketType:      packet.PacketTypeProof,
		Context:         packet.ContextLRProof,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Hops:            7,
		Data:            bytes.Repeat([]byte{0xCD}, identity.SigLength/8+KeySize),
	}
	err = l.ValidateLinkProof(proof, nil)
	if err == nil {
		t.Fatal("expected signature failure")
	}
	if strings.Contains(err.Error(), "hop count mismatch") {
		t.Fatalf("hop gate should allow PATHFINDER_M, got %v", err)
	}
}
