// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// relayIface is a tracking NetworkInterface used by the relay tests; it
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

// TestLinkRelayBidirectional drives a synthetic LINKREQUEST through the
// relay table and then replays a link data packet (matching the link
// id) from the opposite direction; the data must be forwarded back out
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

	raw := make([]byte, 0, 2+16+1+4)
	raw = append(raw, 0x00, 0x05)
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextNone)
	raw = append(raw, []byte{0x01, 0x02, 0x03, 0x04}...)

	if !tr.forwardLinkData(raw, in) {
		t.Fatal("forwardLinkData returned false on known link id (in->out direction)")
	}
	if got := out.snapshot(); len(got) != 1 {
		t.Fatalf("expected 1 packet forwarded out, got %d", len(got))
	}

	if !tr.forwardLinkData(raw, out) {
		t.Fatal("forwardLinkData returned false on known link id (out->in direction)")
	}
	if got := in.snapshot(); len(got) != 1 {
		t.Fatalf("expected 1 packet forwarded in, got %d", len(got))
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
// RequestPath; here we only assert that exactly the non-excluded
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
	if err := tr.RequestPath(destHash, "out", nil, false); err != nil {
		t.Fatalf("second RequestPath: %v", err)
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
