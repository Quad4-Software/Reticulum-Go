// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// TestBughuntUnsignedDATAPathResponseDoesNotPoisonPaths ensures Plain DATA
// with ContextPathResponse cannot inject path-table entries. Real path
// responses are signed announces verified in handleAnnouncePacket.
func TestBughuntUnsignedDATAPathResponseDoesNotPoisonPaths(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.EnableTransport = true
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	iface := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest := make([]byte, 16)
	for i := range dest {
		dest[i] = byte(i + 1)
	}
	attackerNext := make([]byte, 16)
	for i := range attackerNext {
		attackerNext[i] = 0xAA
	}

	flags := byte((packet.DestinationPlain << 2) | packet.PacketTypeData)
	payload := append(append(append([]byte(nil), dest...), make([]byte, 16)...), 1)
	payload = append(payload, attackerNext...)
	raw := append([]byte{flags, 0}, dest...)
	raw = append(raw, packet.ContextPathResponse)
	raw = append(raw, payload...)

	tr.HandlePacket(raw, iface)
	time.Sleep(20 * time.Millisecond)

	if tr.HasPath(dest) {
		t.Fatalf("unsigned DATA PATH_RESPONSE poisoned path table: next=%x", tr.NextHop(dest))
	}
}

// TestBughuntPathNextHopOwnsBytes ensures path NextHop is not aliased to the
// inbound packet buffer.
func TestBughuntPathNextHopOwnsBytes(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.EnableTransport = true
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	iface := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest := make([]byte, 16)
	next := make([]byte, 16)
	for i := range dest {
		dest[i] = byte(i)
		next[i] = byte(0xF0 + i)
	}
	origNext := append([]byte(nil), next...)
	tr.UpdatePath(dest, next, "wan", 2)

	for i := range next {
		next[i] = 0x00
	}
	got := tr.NextHop(dest)
	if string(got) != string(origNext) {
		t.Fatalf("NextHop aliased buffer: got %x want %x", got, origNext)
	}
}
