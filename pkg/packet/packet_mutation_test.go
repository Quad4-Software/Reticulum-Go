// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func TestPackHeaderType2RequiresTransportID(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("x"),
	}
	if err := p.Pack(); err == nil {
		t.Fatal("expected error when header type 2 has no transport ID")
	}
}

func TestPackReusesRawBufferWhenCapacitySufficient(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("small"),
		Raw:             make([]byte, 0, MTU),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	capBefore := cap(p.Raw)
	p.Data = append(p.Data, []byte("more")...)
	p.Packed = false
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	if cap(p.Raw) != capBefore {
		t.Fatalf("expected Raw capacity reuse, got cap %d want %d", cap(p.Raw), capBefore)
	}
}

func TestUnpackHeaderType2ExactBoundary(t *testing.T) {
	dest := randomBytes(16)
	tid := randomBytes(16)
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportID:     tid,
		DestinationHash: dest,
		Context:         ContextNone,
		Data:            []byte("ok"),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}

	short := append([]byte(nil), p.Raw[:2*TruncatedHashLength+MinPacketSize-1]...)
	if err := (&Packet{Raw: short}).Unpack(); err == nil {
		t.Fatal("expected unpack error for short header type 2 packet")
	}

	ht1Short := append([]byte{0x00, 0x00}, dest[:TruncatedHashLength-1]...)
	if err := (&Packet{Raw: ht1Short}).Unpack(); err == nil {
		t.Fatal("expected unpack error for short header type 1 packet")
	}
}

func TestHashablePreimageLenWithShortRaw(t *testing.T) {
	ht1 := &Packet{HeaderType: HeaderType1, Raw: []byte{0x00, 0x01}}
	if n := ht1.hashablePreimageLen(); n != 1 {
		t.Fatalf("ht1 short preimage len=%d want 1", n)
	}

	ht2 := &Packet{HeaderType: HeaderType2, Raw: make([]byte, TruncatedHashLength+2)}
	if n := ht2.hashablePreimageLen(); n != 1 {
		t.Fatalf("ht2 short preimage len=%d want 1", n)
	}

	ht1.Raw = append([]byte{0x00, 0x01}, randomBytes(8)...)
	if n := ht1.hashablePreimageLen(); n != 1+8 {
		t.Fatalf("ht1 preimage len=%d want %d", n, 1+8)
	}

	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportID:     randomBytes(16),
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            randomBytes(32),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	n := p.hashablePreimageLen()
	if n <= 1 {
		t.Fatalf("packed ht2 preimage len=%d", n)
	}
	buf := make([]byte, 0, n)
	hb := p.hashableInto(buf)
	if len(hb) != n {
		t.Fatalf("hashable bytes=%d want %d", len(hb), n)
	}
}

func TestNextRawWireCap(t *testing.T) {
	if got := nextRawWireCap(MTU + 1); got != MTU+1 {
		t.Fatalf("large need cap=%d want %d", got, MTU+1)
	}
	if got := nextRawWireCap(100); got != 128 {
		t.Fatalf("aligned cap=%d want 128", got)
	}
	if got := nextRawWireCap(MTU); got != MTU {
		t.Fatalf("mtu cap=%d want %d", got, MTU)
	}
}

func TestUpdateHashLargePreimage(t *testing.T) {
	p := &Packet{
		HeaderType: HeaderType1,
		Raw:        make([]byte, MTU+50),
	}
	p.Raw[0] = 0x00
	p.Raw[1] = 0x00
	for i := 2; i < len(p.Raw); i++ {
		p.Raw[i] = byte(i)
	}
	p.updateHash()
	if len(p.PacketHash) == 0 {
		t.Fatal("expected hash for large preimage")
	}
}

func TestTruncatedHashLength(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("hash"),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	th := p.TruncatedHash()
	if len(th) != TruncatedHashLength {
		t.Fatalf("truncated hash len=%d want %d", len(th), TruncatedHashLength)
	}
	if !bytes.Equal(th, p.GetHash()[:TruncatedHashLength]) {
		t.Fatal("truncated hash mismatch")
	}
}

func TestSerializePacksWhenNeeded(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("serialize"),
	}
	raw, err := p.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty serialize output")
	}
	if !bytes.Equal(p.DestinationHash, p.Addresses) {
		t.Fatal("Serialize should set Addresses to destination hash")
	}
}

func TestNewAnnouncePacketAppDataFormats(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest := id.Hash()
	transport := randomBytes(16)

	nodePkt, err := NewAnnouncePacket(dest, id, []byte{0x93, 0x01, 0x02, 0x03}, transport)
	if err != nil {
		t.Fatalf("node announce: %v", err)
	}
	if nodePkt.PacketType != PacketTypeAnnounce || nodePkt.HeaderType != HeaderType2 {
		t.Fatalf("unexpected node packet type/header: %d/%d", nodePkt.PacketType, nodePkt.HeaderType)
	}

	name := []byte("peer.one")
	peerApp := append([]byte{0x92, 0xc4, byte(len(name))}, name...)
	peerPkt, err := NewAnnouncePacket(dest, id, peerApp, transport)
	if err != nil {
		t.Fatalf("peer announce: %v", err)
	}
	if len(peerPkt.Data) == 0 {
		t.Fatal("peer announce has empty data")
	}

	fallbackPkt, err := NewAnnouncePacket(dest, id, []byte{0x01}, transport)
	if err != nil {
		t.Fatalf("fallback announce: %v", err)
	}
	if len(fallbackPkt.Data) == 0 {
		t.Fatal("fallback announce has empty data")
	}

	truncatedNameApp := []byte{0x92, 0xc4, 20, 'x'}
	truncPkt, err := NewAnnouncePacket(dest, id, truncatedNameApp, transport)
	if err != nil {
		t.Fatalf("truncated-name announce: %v", err)
	}
	if len(truncPkt.Data) == 0 {
		t.Fatal("truncated-name announce has empty data")
	}

	shortNodeApp := []byte{0x93, 0x01}
	shortPkt, err := NewAnnouncePacket(dest, id, shortNodeApp, transport)
	if err != nil {
		t.Fatalf("short node-prefix announce: %v", err)
	}
	if len(shortPkt.Data) == 0 {
		t.Fatal("short node-prefix announce has empty data")
	}

	exactPeerApp := []byte{0x92, 0xc4, 1, 'z'}
	exactPkt, err := NewAnnouncePacket(dest, id, exactPeerApp, transport)
	if err != nil {
		t.Fatalf("exact peer announce: %v", err)
	}
	if len(exactPkt.Data) == 0 {
		t.Fatal("exact peer announce has empty data")
	}
}
