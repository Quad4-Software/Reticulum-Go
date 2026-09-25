// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// Regression: a proof arriving on the wrong interface must not consume the
// single-use reverse-table entry. The real proof may still arrive on the
// correct outbound interface afterwards.
func TestReverseProofWrongIfaceKeepsEntry(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })

	wan := newRelayIface("wan")
	rogue := newRelayIface("rogue")
	client := newRelayIface("local")
	client.Type = common.IFTypeUnix
	_ = tr.RegisterInterface("wan", wan)
	_ = tr.RegisterInterface("rogue", rogue)
	_ = tr.RegisterInterface("local", client)

	th := bytes.Repeat([]byte{0x11}, 16)
	tr.reverseTable.put(th, &ReverseEntry{
		ReceivedIface: client,
		OutboundIface: wan,
		Timestamp:     time.Now(),
	})

	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeProof & packet.HeaderMaskPacketType
	proofRaw := make([]byte, 0, 2+16+1+8)
	proofRaw = append(proofRaw, flags, 0x01)
	proofRaw = append(proofRaw, th...)
	proofRaw = append(proofRaw, packet.ContextNone)
	proofRaw = append(proofRaw, []byte{1, 2, 3, 4, 5, 6, 7, 8}...)

	proof := &packet.Packet{Raw: proofRaw}
	if err := proof.Unpack(); err != nil {
		t.Fatalf("unpack proof: %v", err)
	}

	// Forged copy on the wrong interface: consumed-but-dropped behavior
	// would kill the real receipt that follows.
	if !tr.forwardReverseProof(proof, rogue) {
		t.Fatal("wrong-iface proof should still be claimed")
	}
	if got := client.snapshot(); len(got) != 0 {
		t.Fatalf("wrong-iface proof delivered %d packets", len(got))
	}

	// The real proof on the outbound interface must still find the entry.
	if !tr.forwardReverseProof(proof, wan) {
		t.Fatal("expected reverse proof forward on correct iface")
	}
	if got := client.snapshot(); len(got) != 1 {
		t.Fatalf("client got %d proofs want 1", len(got))
	}
}

// Regression: HT2 link data carries the link id at raw[18:34], not [2:18].
// Forwarding by fixed offset dropped every multi-hop Python link packet.
func TestForwardLinkDataHT2(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	linkID := bytes.Repeat([]byte{0x42}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		ReceivedIface: in,
		NextHopIface:  out,
		TakenHops:     1,
		RemainingHops: 1,
		Validated:     true,
		Timestamp:     time.Now(),
	})

	// HT2: flags transport|link|data, transport id at [2:18], link id at [18:34].
	flags := byte(0)
	flags |= (packet.HeaderType2 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationTransport << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationLink << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeData & packet.HeaderMaskPacketType
	raw := make([]byte, 0, 2+16+16+1+4)
	// One hop taken so far means zero additional hops on the wire.
	raw = append(raw, flags, 0x00)
	raw = append(raw, bytes.Repeat([]byte{0x77}, 16)...) // transport id
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextNone)
	raw = append(raw, []byte{0xAA, 0xBB, 0xCC, 0xDD}...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack ht2: %v", err)
	}
	if !tr.forwardLinkData(pkt.DestinationHash, raw, in) {
		t.Fatal("HT2 link data was not relayed")
	}
	got := out.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 relayed packet, got %d", len(got))
	}
	if got[0][1] != 1 {
		t.Fatalf("hops not incremented: got %d want 1", got[0][1])
	}
}

// Regression: forged link requests must not grow the relay table forever.
func TestLinkRelayTableCap(t *testing.T) {
	lt := newLinkRelayTable()
	e := &LinkRelayEntry{Timestamp: time.Now()}
	for i := 0; i < maxLinkRelayEntries+64; i++ {
		var id [16]byte
		id[0] = byte(i >> 8)
		id[1] = byte(i)
		lt.put(id[:], &LinkRelayEntry{Timestamp: time.Now(), Validated: false})
	}
	lt.mu.RLock()
	n := len(lt.entries)
	lt.mu.RUnlock()
	if n > maxLinkRelayEntries {
		t.Fatalf("link relay table %d exceeds cap %d", n, maxLinkRelayEntries)
	}
	if e == nil {
		t.Fatal("unreachable")
	}
}

// Regression: the reverse table is bounded too; oldest entries evict first.
func TestReverseTableCap(t *testing.T) {
	rt := &reverseTable{entries: make(map[hash16]*ReverseEntry)}
	for i := 0; i < maxReverseEntries+64; i++ {
		var id [16]byte
		id[0] = byte(i >> 8)
		id[1] = byte(i)
		rt.put(id[:], &ReverseEntry{Timestamp: time.Now()})
	}
	rt.mu.Lock()
	n := len(rt.entries)
	rt.mu.Unlock()
	if n > maxReverseEntries {
		t.Fatalf("reverse table %d exceeds cap %d", n, maxReverseEntries)
	}
}
