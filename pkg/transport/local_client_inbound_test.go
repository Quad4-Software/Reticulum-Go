// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

// Inbound non-announce traffic for a destination owned by a shared-instance
// local client (path entry hops == 0 pointing at the client interface) must
// be forwarded to that client interface, mirroring Python Transport.py's
// for_local_client rule. Regression test for the owner-side forwarding gap
// behind TestMatrix_DeliverySharedInstance.
func TestLocalClientInboundForwarding(t *testing.T) {
	tr := NewTransport(unstampedTestConfig(true))
	defer func() { _ = tr.Close() }()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	destHash := randomDestHash(77)

	// seed the path entry the way a client announce does: zero hops,
	// interface = the spawned local client
	tr.mutex.Lock()
	tr.paths[pathMapKey(destHash)] = &common.Path{
		Interface: lc,
		HopCount:  0,
		NextHop:   destHash,
	}
	tr.mutex.Unlock()

	// build a HeaderType1 data packet for destHash: flags, hops, dest, payload
	raw := make([]byte, 0, 64)
	raw = append(raw, 0x00) // header type 1, broadcast, dest type single, packet type data
	raw = append(raw, 0x01) // hops = 1
	raw = append(raw, destHash...)
	raw = append(raw, 0x00) // context
	raw = append(raw, []byte("inbound-for-local-client")...)

	direct := tr.forwardInboundToLocalClient(&packet.Packet{
		PacketType:      packet.PacketTypeData,
		DestinationHash: append([]byte(nil), destHash...),
	}, raw, wan)
	if !direct {
		t.Fatal("direct forwardInboundToLocalClient call returned false")
	}
	if len(lc.snapshot()) != 1 {
		t.Fatalf("direct call: expected 1 forward, got %d", len(lc.snapshot()))
	}
	lc.mu.Lock()
	lc.sent = nil
	lc.mu.Unlock()

	tr.HandlePacketBlocking(raw, wan)

	var sent [][]byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		sent = lc.snapshot()
		if len(sent) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(sent) != 1 {
		t.Fatalf("expected exactly 1 packet forwarded to local client, got %d", len(sent))
	}
	if len(sent[0]) != len(raw) {
		t.Errorf("forwarded packet length %d != original %d", len(sent[0]), len(raw))
	}

	// the packet must NOT be echoed back onto the WAN interface
	for _, out := range wan.snapshot() {
		if len(out) == len(raw) {
			t.Error("packet echoed back onto receiving interface")
		}
	}
}

// Packets received from a local client must never be forwarded back to a
// local client by the inbound rule (loop prevention).
func TestLocalClientInboundNoLoop(t *testing.T) {
	tr := NewTransport(unstampedTestConfig(true))
	defer func() { _ = tr.Close() }()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}

	destHash := randomDestHash(78)
	tr.mutex.Lock()
	tr.paths[pathMapKey(destHash)] = &common.Path{
		Interface: lc,
		HopCount:  0,
		NextHop:   destHash,
	}
	tr.mutex.Unlock()

	flags2 := byte(0)
	flags2 |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags2 |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags2 |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags2 |= packet.PacketTypeData & packet.HeaderMaskPacketType
	raw := make([]byte, 0, 64)
	raw = append(raw, flags2, 0x01)
	raw = append(raw, destHash...)
	raw = append(raw, 0x00)
	raw = append(raw, []byte("loop-check")...)

	tr.HandlePacketBlocking(raw, lc)

	for _, out := range lc.snapshot() {
		if len(out) == len(raw) {
			t.Error("packet from local client forwarded back to local client")
		}
	}
}

// Announces must not take the inbound forwarding path.
func TestLocalClientInboundSkipsAnnounce(t *testing.T) {
	tr := NewTransport(unstampedTestConfig(true))
	defer func() { _ = tr.Close() }()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	destHash := randomDestHash(79)
	tr.mutex.Lock()
	tr.paths[pathMapKey(destHash)] = &common.Path{
		Interface: lc,
		HopCount:  0,
		NextHop:   destHash,
	}
	tr.mutex.Unlock()

	// announce packet type is 0x01 in low bits
	raw := make([]byte, 0, 64)
	raw = append(raw, byte(packet.PacketTypeAnnounce))
	raw = append(raw, 0x01)
	raw = append(raw, destHash...)
	raw = append(raw, make([]byte, 40)...)

	before := len(lc.snapshot())
	tr.forwardInboundToLocalClient(&packet.Packet{
		PacketType:      packet.PacketTypeAnnounce,
		DestinationHash: destHash,
	}, raw, wan)
	if len(lc.snapshot()) != before {
		t.Error("announce forwarded through inbound local-client path")
	}
}
