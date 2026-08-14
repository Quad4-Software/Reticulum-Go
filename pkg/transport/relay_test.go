// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/packet"
)

// relayIface is a tracking NetworkInterface used by the relay tests. It

// captures every Send so the test can assert what the transport pushed
// out over the wire and on which interface.
type relayIface struct {
	common.BaseInterface
	mu   sync.Mutex
	sent [][]byte
}

func newRelayIface(name string) *relayIface {
	r := &relayIface{
		BaseInterface: common.NewBaseInterface(name, common.IFTypeTCP, true),
	}
	r.Enable()
	return r
}

func (r *relayIface) Send(data []byte, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	r.sent = append(r.sent, cp)
	return nil
}

func (r *relayIface) ProcessOutgoing(data []byte) error { return r.Send(data, "") }

func (r *relayIface) snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.sent))
	for i, p := range r.sent {
		c := make([]byte, len(p))
		copy(c, p)
		out[i] = c
	}
	return out
}

func mustIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	return id
}

// buildHT2Packet hand-rolls a HeaderType2 data packet so the relay
// path can be exercised without going through the full wire-format packet
// builder.
func buildHT2Packet(transportID, destHash []byte, hops byte, payload []byte) []byte {
	flags := byte(0)
	flags |= (packet.HeaderType2 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationTransport << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeData & packet.HeaderMaskPacketType

	out := make([]byte, 0, 2+16+16+1+len(payload))
	out = append(out, flags, hops)
	out = append(out, transportID...)
	out = append(out, destHash...)
	out = append(out, packet.ContextNone)
	out = append(out, payload...)
	return out
}

// TestForwardTransportPacketRewritesNextHop verifies the HeaderType2
// transit path: when we receive a packet whose TransportID equals our
// identity hash and the destination has hops > 1 in our path table, the
// outbound copy must contain the next hop transport id (not ours) and
// must land on the corresponding interface.
func TestForwardTransportPacketRewritesNextHop(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id := mustIdentity(t)
	tr.SetIdentity(id)

	in := newRelayIface("in")
	out := newRelayIface("out")
	if err := tr.RegisterInterface("in", in); err != nil {
		t.Fatalf("register in: %v", err)
	}
	if err := tr.RegisterInterface("out", out); err != nil {
		t.Fatalf("register out: %v", err)
	}

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	tr.UpdatePath(destHash, nextHop, "out", 2)

	raw := buildHT2Packet(id.Hash(), destHash, 0, []byte{0x01, 0x02})
	tr.HandlePacket(raw, in)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(out.snapshot()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := out.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 forwarded packet on out, got %d", len(got))
	}
	pkt := got[0]
	if len(pkt) != len(raw) {
		t.Fatalf("forwarded packet length mismatch: got %d want %d", len(pkt), len(raw))
	}
	if pkt[1] != 1 {
		t.Errorf("hops not incremented: got %d want 1", pkt[1])
	}
	if !bytes.Equal(pkt[2:18], nextHop) {
		t.Errorf("transport id not rewritten: got %x want %x", pkt[2:18], nextHop)
	}
	if !bytes.Equal(pkt[18:34], destHash) {
		t.Errorf("destination hash mutated: got %x want %x", pkt[18:34], destHash)
	}

	if len(in.snapshot()) != 0 {
		t.Errorf("packet leaked back onto receiving interface: %x", in.snapshot())
	}
}

// TestForwardTransportPacketLastHopStripsHeader verifies that when the
// final hop is one away (path.HopCount == 1) we collapse the packet to
// HeaderType1 + broadcast so the destination sees a normal directly-
// addressed packet, matching Transport.inbound's remaining_hops==1
// branch.
func TestForwardTransportPacketLastHopStripsHeader(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id := mustIdentity(t)
	tr.SetIdentity(id)

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	destHash := bytes.Repeat([]byte{0xCC}, 16)
	tr.UpdatePath(destHash, destHash, "out", 1)

	raw := buildHT2Packet(id.Hash(), destHash, 0, []byte{0xDE, 0xAD})
	tr.HandlePacket(raw, in)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(out.snapshot()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := out.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 forwarded packet, got %d", len(got))
	}
	pkt := got[0]
	if want := len(raw) - 16; len(pkt) != want {
		t.Fatalf("expected stripped packet length %d, got %d", want, len(pkt))
	}
	headerType := (pkt[0] & packet.HeaderMaskHeaderType) >> 6
	if headerType != packet.HeaderType1 {
		t.Errorf("expected HeaderType1 after strip, got %d", headerType)
	}
	transportType := (pkt[0] & packet.HeaderMaskTransportType) >> 4
	if transportType != packet.PropagationBroadcast {
		t.Errorf("expected broadcast transport type after strip, got %d", transportType)
	}
	if !bytes.Equal(pkt[2:18], destHash) {
		t.Errorf("dest hash misplaced after strip: got %x want %x", pkt[2:18], destHash)
	}
}

// TestForwardTransportPacketDisabledByConfig confirms the EnableTransport
// gate: when the local instance is not a transport node, HeaderType2
// packets addressed to our transport id are dropped (returns true,
// nothing forwarded) instead of being silently relayed.
func TestForwardTransportPacketDisabledByConfig(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()
	id := mustIdentity(t)
	tr.SetIdentity(id)

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	tr.UpdatePath(destHash, nextHop, "out", 2)

	raw := buildHT2Packet(id.Hash(), destHash, 0, []byte{0x09})
	tr.HandlePacket(raw, in)

	time.Sleep(150 * time.Millisecond)
	if n := len(out.snapshot()); n != 0 {
		t.Fatalf("transport relay leaked %d packets while disabled", n)
	}
}

// TestForwardTransportPacketIgnoresOtherTransportID verifies that a
// HeaderType2 packet whose transport id does not match ours falls
// through to the local-destination handling path (which itself will
// drop it when no destination exists). The relay must not transmit
// anything on outbound interfaces in that case.
func TestForwardTransportPacketIgnoresOtherTransportID(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	other := bytes.Repeat([]byte{0xEE}, 16)
	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	tr.UpdatePath(destHash, nextHop, "out", 2)

	raw := buildHT2Packet(other, destHash, 0, []byte{0xFE})
	tr.HandlePacket(raw, in)

	time.Sleep(100 * time.Millisecond)
	if n := len(out.snapshot()); n != 0 {
		t.Fatalf("relay forwarded packet not addressed to us: %d packets", n)
	}
}

// TestRelayBridgedLinkRequestForwardsHT1 verifies HeaderType1 link requests are
// relayed across a known path when the destination is not local.
func TestRelayBridgedLinkRequestForwardsHT1(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	tr.UpdatePath(destHash, destHash, "out", 1)

	requestData := bytes.Repeat([]byte{0x42}, packet.LinkRequestECPubSize+3)
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType

	raw := make([]byte, 0, 2+16+len(requestData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, destHash...)
	raw = append(raw, requestData...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if !tr.relayBridgedLinkRequest(pkt, raw, in) {
		t.Fatal("relayBridgedLinkRequest returned false")
	}
	got := out.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 forwarded link request, got %d", len(got))
	}
	if got[0][1] != 0x01 {
		t.Fatalf("non-local bridged LR hops = %d, want 1", got[0][1])
	}
	if got[0][0]>>6 != packet.HeaderType1 {
		t.Fatalf("direct path bridged LR header = %d, want HT1", got[0][0]>>6)
	}
	linkID := packet.LinkIDFromLinkRequest(pkt)
	entry, ok := tr.linkTable.get(linkID)
	if !ok {
		t.Fatal("missing link relay entry")
	}
	if entry.TakenHops != 1 {
		t.Fatalf("TakenHops = %d, want 1 for non-local LR", entry.TakenHops)
	}
}

// TestRelayBridgedLinkRequestInsertsHT2ForMultiHop ensures multi-hop bridged
// link requests are wrapped with HeaderType2 and the next transport hop so
// mesh peers will accept and forward them.
func TestRelayBridgedLinkRequestInsertsHT2ForMultiHop(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	tr.UpdatePath(destHash, nextHop, "out", 3)

	requestData := bytes.Repeat([]byte{0x42}, packet.LinkRequestECPubSize+3)
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType

	raw := make([]byte, 0, 2+16+len(requestData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, destHash...)
	raw = append(raw, requestData...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if !tr.relayBridgedLinkRequest(pkt, raw, in) {
		t.Fatal("relayBridgedLinkRequest returned false")
	}
	got := out.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 forwarded link request, got %d", len(got))
	}
	fwd := got[0]
	if fwd[0]>>6 != packet.HeaderType2 {
		t.Fatalf("multi-hop bridged LR header = %d, want HT2", fwd[0]>>6)
	}
	if (fwd[0]&packet.HeaderMaskTransportType)>>4 != packet.PropagationTransport {
		t.Fatalf("multi-hop bridged LR transport type unset")
	}
	if fwd[1] != 0x01 {
		t.Fatalf("multi-hop bridged LR hops = %d, want 1", fwd[1])
	}
	if !bytes.Equal(fwd[2:18], nextHop) {
		t.Fatalf("transport id = %x, want %x", fwd[2:18], nextHop)
	}
	if !bytes.Equal(fwd[18:34], destHash) {
		t.Fatalf("dest hash = %x, want %x", fwd[18:34], destHash)
	}
	if len(fwd) != len(raw)+16 {
		t.Fatalf("forwarded len = %d, want %d", len(fwd), len(raw)+16)
	}

	unpacked := &packet.Packet{Raw: fwd}
	if err := unpacked.Unpack(); err != nil {
		t.Fatalf("unpack forwarded: %v", err)
	}
	if unpacked.PacketType != packet.PacketTypeLinkReq {
		t.Fatalf("packet type = %d, want LinkReq", unpacked.PacketType)
	}
	linkID := packet.LinkIDFromLinkRequest(pkt)
	if _, ok := tr.linkTable.get(linkID); !ok {
		t.Fatal("missing link relay entry")
	}
}

// TestLinkProofRelayViaLinkTable verifies forged LR proofs are not forwarded
// through the link relay table (signature gate).
func TestLinkProofRelayViaLinkTable(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")

	linkID := bytes.Repeat([]byte{0x77}, 16)
	destHash := bytes.Repeat([]byte{0x88}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHop:         bytes.Repeat([]byte{0xBB}, 16),
		NextHopIface:    out,
		ReceivedIface:   in,
		RemainingHops:   1,
		TakenHops:       0,
		DestinationHash: destHash,
		ProofTimeout:    time.Now().Add(time.Hour),
		Timestamp:       time.Now(),
		OriginalLinkID:  linkID,
	})

	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationLink << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeProof & packet.HeaderMaskPacketType

	raw := make([]byte, 0, 2+16+1+4)
	raw = append(raw, flags, 0x00)
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextLRProof)
	raw = append(raw, []byte{0x01, 0x02, 0x03, 0x04}...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack proof: %v", err)
	}

	tr.handleProofPacket(pkt, out)
	if got := in.snapshot(); len(got) != 0 {
		t.Fatalf("forged LRPROOF must not relay, got %d packets", len(got))
	}
}

// TestRecordLinkRelayUsesWireLinkID verifies relay registration keys match
// endpoint link IDs derived from the link request wire format.
func TestRecordLinkRelayUsesWireLinkID(t *testing.T) {
	destHash := bytes.Repeat([]byte{0xAA}, 16)
	requestData := bytes.Repeat([]byte{0x42}, packet.LinkRequestECPubSize+3)

	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType

	raw := make([]byte, 0, 2+16+len(requestData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, destHash...)
	raw = append(raw, requestData...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack link request: %v", err)
	}

	want := packet.LinkIDFromLinkRequest(pkt)
	if len(want) != 16 {
		t.Fatalf("link id len=%d want 16", len(want))
	}

	old := sha256LinkID(destHash, requestData)
	if bytes.Equal(want, old) {
		t.Fatal("test vector accidentally matches deprecated sha256(dest+data) algorithm")
	}

	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)
	tr.UpdatePath(destHash, destHash, "out", 1)

	tr.recordLinkRelay(pkt, raw, in, &common.Path{
		NextHop:   destHash,
		Interface: out,
		HopCount:  1,
	}, int(linkRelayAccountedHops(pkt.Hops, isLocalClientInterface(in))))

	entry, ok := tr.linkTable.get(want)
	if !ok {
		t.Fatalf("link table missing entry for wire link id %x", want)
	}
	if entry == nil || entry.NextHopIface != out || entry.ReceivedIface != in {
		t.Fatalf("unexpected relay entry: %+v", entry)
	}
}

func TestRecordLinkRelayProofTimeoutUsesOutboundBitrate(t *testing.T) {
	destHash := bytes.Repeat([]byte{0x31}, 16)
	requestData := bytes.Repeat([]byte{0x42}, packet.LinkRequestECPubSize+3)
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType
	raw := make([]byte, 0, 2+16+len(requestData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, destHash...)
	raw = append(raw, requestData...)
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack link request: %v", err)
	}

	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	in := newRelayIface("in")
	out := &bitrateIface{}
	out.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
	out.Online = true
	out.bitrate = 125
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("radio", out)

	before := time.Now()
	tr.recordLinkRelay(pkt, raw, in, &common.Path{
		NextHop:   destHash,
		Interface: out,
		HopCount:  1,
	}, 1)
	linkID := packet.LinkIDFromLinkRequest(pkt)
	entry, ok := tr.linkTable.get(linkID)
	if !ok || entry == nil {
		t.Fatal("missing relay entry")
	}
	want := LinkProofTimeoutPerHop + ExtraLinkProofTimeout(out)
	got := entry.ProofTimeout.Sub(before)
	if got < want-50*time.Millisecond || got > want+time.Second {
		t.Fatalf("proof timeout delta %s, want ~%s", got, want)
	}
}

func sha256LinkID(dest, data []byte) []byte {
	h := sha256.Sum256(append(append([]byte(nil), dest...), data...))
	return h[:16]
}

// TestLinkRelayBidirectional drives a synthetic LINKREQUEST through the
// relay table and then replays a link data packet (matching the link
// id) from the opposite direction. The data must be forwarded back out
// the original receiving interface.
func TestLinkRelayBidirectional(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")

	linkID := bytes.Repeat([]byte{0x77}, 16)
	entry := &LinkRelayEntry{
		NextHop:        bytes.Repeat([]byte{0xBB}, 16),
		NextHopIface:   out,
		ReceivedIface:  in,
		RemainingHops:  1,
		TakenHops:      1,
		ProofTimeout:   time.Now().Add(time.Hour),
		Timestamp:      time.Now(),
		OriginalLinkID: linkID,
	}
	tr.linkTable.put(linkID, entry)

	// Non-local ingress: wire hops N become accounted N+1, which must match TakenHops.
	raw := make([]byte, 0, 2+16+1+4)
	raw = append(raw, 0x00, 0x00)
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextNone)
	raw = append(raw, []byte{0x01, 0x02, 0x03, 0x04}...)
	origHops := raw[1]

	if !tr.forwardLinkData(raw, in) {
		t.Fatal("forwardLinkData returned false on known link id (in->out direction)")
	}
	if raw[1] != origHops {
		t.Fatalf("forwardLinkData mutated caller buffer hops: got %d want %d", raw[1], origHops)
	}
	gotOut := out.snapshot()
	if len(gotOut) != 1 {
		t.Fatalf("expected 1 packet forwarded out, got %d", len(gotOut))
	}
	if gotOut[0][1] != 0x01 {
		t.Fatalf("forwarded hops = %d, want 1 (accounted)", gotOut[0][1])
	}

	// Return path from next-hop iface: wire 0 → accounted 1 == RemainingHops.
	ret := make([]byte, 0, 2+16+1+4)
	ret = append(ret, 0x00, 0x00)
	ret = append(ret, linkID...)
	ret = append(ret, packet.ContextNone)
	ret = append(ret, []byte{0x01, 0x02, 0x03, 0x04}...)

	if !tr.forwardLinkData(ret, out) {
		t.Fatal("forwardLinkData returned false on known link id (out->in direction)")
	}
	gotIn := in.snapshot()
	if len(gotIn) != 1 {
		t.Fatalf("expected 1 packet forwarded in, got %d", len(gotIn))
	}
	if gotIn[0][1] != 0x01 {
		t.Fatalf("return hops = %d, want 1", gotIn[0][1])
	}
}

// TestLocalClientLinkHopSpoofing ensures shared-instance local clients do not
// consume a hop on link request or identify relay, matching Python Transport.
func TestLocalClientLinkHopSpoofing(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("local-client")
	in.Type = common.IFTypeUnix
	out := newRelayIface("wan")
	_ = tr.RegisterInterface("local-client", in)
	_ = tr.RegisterInterface("wan", out)

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	tr.UpdatePath(destHash, destHash, "wan", 4)

	requestData := bytes.Repeat([]byte{0x42}, packet.LinkRequestECPubSize+3)
	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationBroadcast << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeLinkReq & packet.HeaderMaskPacketType

	raw := make([]byte, 0, 2+16+len(requestData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, destHash...)
	raw = append(raw, requestData...)

	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if !tr.relayBridgedLinkRequest(pkt, raw, in) {
		t.Fatal("relayBridgedLinkRequest returned false")
	}
	fwd := out.snapshot()
	if len(fwd) != 1 {
		t.Fatalf("expected 1 forwarded link request, got %d", len(fwd))
	}
	if fwd[0][1] != 0x00 {
		t.Fatalf("local-client link request hops = %d, want 0 (no hop consumed)", fwd[0][1])
	}

	linkID := packet.LinkIDFromLinkRequest(pkt)
	entry, ok := tr.linkTable.get(linkID)
	if !ok {
		t.Fatal("missing link relay entry")
	}
	if entry.TakenHops != 0 {
		t.Fatalf("TakenHops = %d, want 0 for local-client LR", entry.TakenHops)
	}
	if entry.RemainingHops != 4 {
		t.Fatalf("RemainingHops = %d, want 4", entry.RemainingHops)
	}

	// Identify / link data from the local client must keep hops=0 and pass TakenHops.
	ident := make([]byte, 0, 2+16+1+4)
	ident = append(ident, 0x00, 0x00)
	ident = append(ident, linkID...)
	ident = append(ident, packet.ContextLinkIdentify)
	ident = append(ident, []byte{0xde, 0xad, 0xbe, 0xef}...)

	out.mu.Lock()
	out.sent = nil
	out.mu.Unlock()

	if !tr.forwardLinkData(ident, in) {
		t.Fatal("forwardLinkData should relay identify from local client")
	}
	identFwd := out.snapshot()
	if len(identFwd) != 1 {
		t.Fatalf("expected identify relayed, got %d", len(identFwd))
	}
	if identFwd[0][1] != 0x00 {
		t.Fatalf("identify hops = %d, want 0", identFwd[0][1])
	}

	// Proof returning from WAN: wire 3 → accounted 4 == RemainingHops.
	proof := make([]byte, 0, 2+16+1+4)
	proof = append(proof, 0x00, 0x03)
	proof = append(proof, linkID...)
	proof = append(proof, packet.ContextLRProof)
	proof = append(proof, []byte{0x01, 0x02, 0x03, 0x04}...)

	if !tr.forwardLinkData(proof, out) {
		t.Fatal("forwardLinkData should relay proof from wan")
	}
	proofFwd := in.snapshot()
	if len(proofFwd) != 1 {
		t.Fatalf("expected proof relayed to local client, got %d", len(proofFwd))
	}
	if proofFwd[0][1] != 0x04 {
		t.Fatalf("proof hops = %d, want 4", proofFwd[0][1])
	}
}

// TestLinkRelayHopMismatchDrops verifies hop-gated link relay drops bad packets.
func TestLinkRelayHopMismatchDrops(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")
	linkID := bytes.Repeat([]byte{0x77}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHopIface:  out,
		ReceivedIface: in,
		RemainingHops: 4,
		TakenHops:     0,
		ProofTimeout:  time.Now().Add(time.Hour),
		Timestamp:     time.Now(),
	})

	raw := make([]byte, 0, 2+16+1)
	raw = append(raw, 0x00, 0x05) // accounted 6 != TakenHops 0
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextNone)

	if !tr.forwardLinkData(raw, in) {
		t.Fatal("should claim known link id even when dropping")
	}
	if n := len(out.snapshot()); n != 0 {
		t.Fatalf("hop mismatch leaked %d packets", n)
	}
}

// TestLinkRelayDisabledByConfig confirms link data forwarding is gated
// on EnableTransport even when an entry exists.
func TestLinkRelayDisabledByConfig(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")

	linkID := bytes.Repeat([]byte{0x77}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHopIface:  out,
		ReceivedIface: in,
		ProofTimeout:  time.Now().Add(time.Hour),
		Timestamp:     time.Now(),
	})

	raw := make([]byte, 0, 2+16+1)
	raw = append(raw, 0x00, 0x00)
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextNone)

	if !tr.forwardLinkData(raw, in) {
		t.Fatal("forwardLinkData should claim packet (drop) when transport disabled")
	}
	if n := len(out.snapshot()) + len(in.snapshot()); n != 0 {
		t.Fatalf("link relay leaked %d packets while transport disabled", n)
	}
}

// TestRebroadcastPathRequestSkipsExcludedIface verifies that
// rebroadcastPathRequest fans out to every enabled interface other
// than the one the request arrived on, and emits a properly-formatted
// Path request packet (header type 1, plain destination,
// transport path.request hash). The exact byte payload of a path
// request is a control plane responsibility we delegate to
// RequestPath. Here we only assert that exactly the non-excluded

// interfaces saw a Send call and the receive interface saw none.
func TestRebroadcastPathRequestSkipsExcludedIface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	in := newRelayIface("in")
	a := newRelayIface("ifaceA")
	b := newRelayIface("ifaceB")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("ifaceA", a)
	_ = tr.RegisterInterface("ifaceB", b)

	destHash := bytes.Repeat([]byte{0x42}, 16)

	tr.rebroadcastPathRequest(destHash, nil, []byte("test-tag-12345678"), in)

	if got := len(in.snapshot()); got != 0 {
		t.Errorf("rebroadcast leaked back to receive interface: %d packets", got)
	}
	if got := len(a.snapshot()); got != 1 {
		t.Errorf("ifaceA expected 1 path-request emission, got %d", got)
	}
	if got := len(b.snapshot()); got != 1 {
		t.Errorf("ifaceB expected 1 path-request emission, got %d", got)
	}
}

// TestRebroadcastPathRequestRespectsTransportFlag confirms that path
// requests are not rebroadcast when transport is disabled.
func TestRebroadcastPathRequestRespectsTransportFlag(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	a := newRelayIface("ifaceA")
	b := newRelayIface("ifaceB")
	_ = tr.RegisterInterface("ifaceA", a)
	_ = tr.RegisterInterface("ifaceB", b)

	destHash := bytes.Repeat([]byte{0x42}, 16)
	tr.rebroadcastPathRequest(destHash, nil, []byte("a-tag"), nil)
	if n := len(a.snapshot()) + len(b.snapshot()); n != 0 {
		t.Fatalf("rebroadcast emitted %d packets while transport disabled", n)
	}
}

// TestRequestPathThrottle exercises the per-destination minimum
// interval (PathRequestMI). The first call schedules a request, the
// second within the window must be suppressed and emit nothing.
func TestRequestPathThrottle(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	out := newRelayIface("out")
	_ = tr.RegisterInterface("out", out)

	destHash := bytes.Repeat([]byte{0x55}, 16)

	if err := tr.RequestPath(destHash, "out", nil, false); err != nil {
		t.Fatalf("first RequestPath: %v", err)
	}
	first := len(out.snapshot())
	if first == 0 {
		t.Fatal("first RequestPath emitted no packet")
	}
	if err := tr.RequestPath(destHash, "out", nil, false); !errors.Is(err, common.ErrPathRequestThrottled) {
		t.Fatalf("second RequestPath = %v, want ErrPathRequestThrottled", err)
	}
	if got := len(out.snapshot()); got != first {
		t.Fatalf("throttled RequestPath still emitted: %d != %d", got, first)
	}
}

// TestRequestPathDoesNotMutateInputHash ensures RequestPath never appends
// onto caller-owned destination-hash storage. This guards against
// side-effects when the caller provides a short slice with spare capacity.
func TestRequestPathDoesNotMutateInputHash(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	out := newRelayIface("out")
	if err := tr.RegisterInterface("out", out); err != nil {
		t.Fatalf("register out: %v", err)
	}

	backing := make([]byte, 32)
	copy(backing[:16], bytes.Repeat([]byte{0x33}, 16))
	destHash := backing[:16]
	before := append([]byte(nil), backing...)

	if err := tr.RequestPath(destHash, "out", nil, false); err != nil {
		t.Fatalf("RequestPath: %v", err)
	}

	if !bytes.Equal(backing, before) {
		t.Fatalf("RequestPath mutated caller-owned destination backing: before=%x after=%x", before, backing)
	}
}

// TestHandleAnnouncePacketRespectsTransportFlag checks that an
// announce received from another node is NOT forwarded onto other
// interfaces when EnableTransport is false. The receiving interface
// itself must not be touched (we never send back the way we came).
func TestHandleAnnouncePacketRespectsTransportFlag(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	// Build a syntactically valid (but signature-invalid) announce
	// header so handleAnnouncePacket reaches its forwarding gate.
	// Signature verification will fail and the function will return
	// before the gate, so we instead drive HandleAnnounce which skips
	// signature checks. That is enough to assert the transport gate.
	id := mustIdentity(t)
	destHash := bytes.Repeat([]byte{0x88}, 16)
	pubKey := id.GetPublicKey()
	if len(pubKey) < 64 {
		t.Skip("identity public key shorter than expected (64 bytes)")
	}
	body := make([]byte, 0)
	body = append(body, 0x00, 0x00)
	body = append(body, destHash...)
	body = append(body, pubKey...)
	body = append(body, bytes.Repeat([]byte{0x00}, 64)...)

	_ = tr.HandleAnnounce(body, in)
	if n := len(out.snapshot()); n != 0 {
		t.Fatalf("announce forwarded while transport disabled: %d packets", n)
	}
}
