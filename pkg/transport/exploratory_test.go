// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/blackhole"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

type countingAnnounceHandler struct {
	wantPathResponses bool
	calls             atomic.Int32
}

func (h *countingAnnounceHandler) AspectFilter() []string { return nil }
func (h *countingAnnounceHandler) ReceivePathResponses() bool {
	return h.wantPathResponses
}
func (h *countingAnnounceHandler) ReceivedAnnounce([]byte, any, []byte, uint8) error {
	h.calls.Add(1)
	return nil
}

func signedAnnounceWithContext(t *testing.T, tr *Transport, id *identity.Identity, ctx byte) (raw, destHash []byte) {
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
	pkt.Context = ctx
	raw, err = pkt.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	raw[1] = 0
	return raw, dest.GetHash()
}

func withFastAnnounceForward(t *testing.T) {
	t.Helper()
	zero := 0.0
	simHooksMu.Lock()
	prev := simPathfinderRW
	simPathfinderRW = &zero
	simHooksMu.Unlock()
	t.Cleanup(func() {
		simHooksMu.Lock()
		simPathfinderRW = prev
		simHooksMu.Unlock()
	})
}

// TestExploratoryBlackholeAnnounceDropped ensures blackholed identities cannot
// inject paths or trigger rebroadcasts (Python Transport blackhole gate).
func TestExploratoryBlackholeAnnounceDropped(t *testing.T) {
	withFastAnnounceForward(t)
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	in := newTrackingIface("in")
	out := newTrackingIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	local, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	blackhole.SetLocalIdentityHash(local.Hash())
	tab := blackhole.New("")
	if _, err := tab.Add(id.Hash(), 0, "exploratory"); err != nil {
		t.Fatal(err)
	}
	tr.SetBlackholeTable(tab)

	handler := &countingAnnounceHandler{wantPathResponses: true}
	tr.RegisterAnnounceHandler(handler)

	raw, dest := signedAnnounceWithContext(t, tr, id, packet.ContextNone)
	tr.HandlePacket(raw, in)
	time.Sleep(80 * time.Millisecond)

	if tr.HasPath(dest) {
		t.Fatal("blackholed announce must not install a path")
	}
	if handler.calls.Load() != 0 {
		t.Fatal("blackholed announce must not notify handlers")
	}
	if n := sentCount(out); n != 0 {
		t.Fatalf("blackholed announce rebroadcasted %d times", n)
	}
}

// TestExploratoryPathResponseAnnounceDoesNotRebroadcast matches Python: PATH_RESPONSE
// may update the path table but must not flood other interfaces.
func TestExploratoryPathResponseAnnounceDoesNotRebroadcast(t *testing.T) {
	withFastAnnounceForward(t)
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	in := newTrackingIface("in")
	out := newTrackingIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	noPR := &countingAnnounceHandler{wantPathResponses: false}
	wantPR := &countingAnnounceHandler{wantPathResponses: true}
	tr.RegisterAnnounceHandler(noPR)
	tr.RegisterAnnounceHandler(wantPR)

	raw, dest := signedAnnounceWithContext(t, tr, id, packet.ContextPathResponse)
	tr.HandlePacket(raw, in)
	time.Sleep(80 * time.Millisecond)

	if !tr.HasPath(dest) {
		t.Fatal("PATH_RESPONSE should still install a path")
	}
	if noPR.calls.Load() != 0 {
		t.Fatal("handler without ReceivePathResponses must not fire")
	}
	if wantPR.calls.Load() != 1 {
		t.Fatalf("path-response handler calls=%d want 1", wantPR.calls.Load())
	}
	if n := sentCount(out); n != 0 {
		t.Fatalf("PATH_RESPONSE must not rebroadcast, got %d sends", n)
	}
}

// TestExploratoryLocalClientAnnounceHopsStayZero ensures shared-instance client
// announces do not inflate path hop counts (Python hops+=1 then hops-=1).
func TestExploratoryLocalClientAnnounceHopsStayZero(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	client := newTrackingIface("local-client")
	client.Type = common.IFTypeUnix
	_ = tr.RegisterInterface("local-client", client)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, dest := signedAnnounceWithContext(t, tr, id, packet.ContextNone)
	raw[1] = 0
	tr.HandlePacket(raw, client)
	time.Sleep(30 * time.Millisecond)

	if !tr.HasPath(dest) {
		t.Fatal("expected path from local-client announce")
	}
	if hops := tr.HopsTo(dest); hops != 0 {
		t.Fatalf("local-client announce hops=%d want 0", hops)
	}
}

// TestExploratoryPlainMultiHopDropped matches Python packet_filter for PLAIN DATA.
func TestExploratoryPlainMultiHopDropped(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	iface := newRelayIface("wan")
	_ = tr.RegisterInterface("wan", iface)

	dest := bytes.Repeat([]byte{0x11}, 16)
	flags := byte((packet.DestinationPlain << 2) | packet.PacketTypeData)
	raw := append([]byte{flags, 2}, dest...)
	raw = append(raw, packet.ContextNone)
	raw = append(raw, []byte("plain-payload")...)

	// Poison attempt: unsigned path-style payload must not install a route
	// and must be dropped before DATA handling when hops account > 1.
	tr.HandlePacket(raw, iface)
	time.Sleep(20 * time.Millisecond)
	if tr.HasPath(dest) {
		t.Fatal("multi-hop PLAIN packet must be dropped")
	}
}

// TestExploratoryDiscoveryPRTagCapKeepsNewest ensures the just-inserted PR tag
// survives eviction at DiscoveryPRTagsCap.
func TestExploratoryDiscoveryPRTagCapKeepsNewest(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	_ = tr.RegisterInterface("wan", wan)

	fill := DiscoveryPRTagsCap
	for i := range fill {
		dest := bytes.Repeat([]byte{byte(i)}, 16)
		tag := bytes.Repeat([]byte{byte(0xA0 + i%16)}, 16)
		payload := append(append([]byte(nil), dest...), tag...)
		tr.handlePathRequest(payload, wan)
	}

	newestDest := bytes.Repeat([]byte{0xEE}, 16)
	newestTag := bytes.Repeat([]byte{0xEF}, 16)
	newestPayload := append(append([]byte(nil), newestDest...), newestTag...)
	tr.handlePathRequest(newestPayload, wan)

	unique := append(append([]byte(nil), newestDest...), newestTag...)
	tr.mutex.RLock()
	kept := tr.discoveryPRTags[string(unique)]
	n := len(tr.discoveryPRTags)
	tr.mutex.RUnlock()
	if !kept {
		t.Fatal("newest discovery PR tag was self-evicted")
	}
	if n > DiscoveryPRTagsCap {
		t.Fatalf("tag map size %d exceeds cap %d", n, DiscoveryPRTagsCap)
	}

	// Replay of newest must still be suppressed.
	before := len(wan.snapshot())
	tr.handlePathRequest(newestPayload, wan)
	after := len(wan.snapshot())
	if after != before {
		t.Fatal("duplicate newest PR was not suppressed after cap eviction")
	}
}

// TestExploratoryLinkRelayUnvalidatedExpiresUnderTraffic ensures unvalidated
// link-table rows die at proof timeout even when transit refreshes Timestamp.
func TestExploratoryLinkRelayUnvalidatedExpiresUnderTraffic(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })

	in := newRelayIface("in")
	out := newRelayIface("out")
	linkID := bytes.Repeat([]byte{0x44}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHop:        bytes.Repeat([]byte{0x55}, 16),
		NextHopIface:   out,
		ReceivedIface:  in,
		RemainingHops:  2,
		TakenHops:      1,
		Validated:      false,
		ProofTimeout:   time.Now().Add(-time.Second),
		Timestamp:      time.Now(),
		OriginalLinkID: linkID,
	})

	ident := make([]byte, 0, 2+16+1+4)
	ident = append(ident, 0x00, 0x01)
	ident = append(ident, linkID...)
	ident = append(ident, packet.ContextLinkIdentify)
	ident = append(ident, []byte{0xde, 0xad, 0xbe, 0xef}...)
	_ = tr.forwardLinkData(ident, in)

	if _, ok := tr.linkTable.get(linkID); !ok {
		t.Fatal("entry vanished before sweep")
	}
	removed := tr.linkTable.sweep(time.Duration(StaleTime) * time.Second)
	if removed != 1 {
		t.Fatalf("sweep removed %d, want 1 unvalidated timed-out entry", removed)
	}
	if _, ok := tr.linkTable.get(linkID); ok {
		t.Fatal("unvalidated link relay survived proof timeout under traffic")
	}
}

// TestAcceptanceNormalAnnounceStillRebroadcasts is the positive counterpart
// to the PATH_RESPONSE exploratory check: ordinary announces must still fan out.
func TestAcceptanceNormalAnnounceStillRebroadcasts(t *testing.T) {
	withFastAnnounceForward(t)
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	in := newTrackingIface("in")
	out := newTrackingIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, dest := signedAnnounceWithContext(t, tr, id, packet.ContextNone)
	tr.HandlePacket(raw, in)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sentCount(out) > 0 && tr.HasPath(dest) {
			return
		}
		time.Sleep(20 * time.Millisecond)
		tr.processDelayedAnnounceJobs()
	}
	t.Fatalf("normal announce path=%v forwards=%d", tr.HasPath(dest), sentCount(out))
}

// TestExploratoryHasPathUsesPathfinderE documents PATHFINDER_E membership vs the
// older PathRequestTTL cull.
func TestExploratoryHasPathUsesPathfinderE(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next"), iface.Name, 1)
	backdatePath(tr, dest, time.Duration(PathRequestTTL+120)*time.Second)
	if !tr.HasPath(dest) {
		t.Fatal("HasPath must keep paths for PATHFINDER_E, not PathRequestTTL")
	}
}

// TestExploratoryReverseTableForwardsReceiptProof ensures multi-hop DATA proofs
// return via reverse_table.
func TestExploratoryReverseTableForwardsReceiptProof(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })
	tr.SetIdentity(mustIdentity(t))

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	dest := bytes.Repeat([]byte{0xAB}, 16)
	nextHop := bytes.Repeat([]byte{0xCD}, 16)
	tr.UpdatePath(dest, nextHop, "out", 2)

	raw := buildHT2Packet(tr.ourTransportID(), dest, 0, []byte("payload-for-proof"))
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if !tr.forwardTransportPacket(pkt, raw, in) {
		t.Fatal("expected transport relay")
	}
	th := pkt.TruncatedHash()
	if _, ok := tr.reverseTable.pop(append([]byte(nil), th...)); !ok {
		// re-put after check by recording again
		t.Fatal("reverse table missing truncated hash after DATA relay")
	}
	tr.recordReverseEntry(pkt, in, out)

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
	if !tr.forwardReverseProof(proof, out) {
		t.Fatal("expected reverse proof forward")
	}
	if got := in.snapshot(); len(got) != 1 {
		t.Fatalf("proof should return on ingress iface, got %d", len(got))
	}
}

// TestExploratoryLRProofTransitRequiresValidSignature ensures forged LRPROOF cannot
// traverse the link relay table, while a correctly signed proof does.
func TestExploratoryLRProofTransitRequiresValidSignature(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })

	in := newRelayIface("in")
	out := newRelayIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In, destination.Single, "reticulum-go", tr, "node")
	if err != nil {
		t.Fatal(err)
	}
	destHash := dest.GetHash()
	identity.Remember(nil, destHash, id.GetPublicKey(), nil)

	linkID := bytes.Repeat([]byte{0x71}, 16)
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHop:         bytes.Repeat([]byte{0x72}, 16),
		NextHopIface:    out,
		ReceivedIface:   in,
		RemainingHops:   1,
		TakenHops:       0,
		DestinationHash: append([]byte(nil), destHash...),
		ProofTimeout:    time.Now().Add(time.Hour),
		Timestamp:       time.Now(),
		OriginalLinkID:  linkID,
	})

	peerPub := make([]byte, lrProofX25519Size)
	for i := range peerPub {
		peerPub[i] = byte(0xA0 + i)
	}
	sigPub := id.GetPublicKey()[lrProofX25519Size : lrProofX25519Size*2]
	signed := append(append(append([]byte(nil), linkID...), peerPub...), sigPub...)
	sig, err := id.Sign(signed)
	if err != nil {
		t.Fatal(err)
	}
	proofData := append(append([]byte(nil), sig...), peerPub...)

	flags := byte(0)
	flags |= (packet.HeaderType1 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.DestinationLink << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeProof & packet.HeaderMaskPacketType
	raw := make([]byte, 0, 2+16+1+len(proofData))
	raw = append(raw, flags, 0x00)
	raw = append(raw, linkID...)
	raw = append(raw, packet.ContextLRProof)
	raw = append(raw, proofData...)
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if !tr.validateAndForwardLRProof(pkt, out) {
		t.Fatal("valid LRPROOF should be handled")
	}
	if got := in.snapshot(); len(got) != 1 {
		t.Fatalf("valid LRPROOF should forward, got %d", len(got))
	}
	entry, ok := tr.linkTable.get(linkID)
	if !ok || !entry.Validated {
		t.Fatal("link relay entry must be marked Validated")
	}

	// Forged proof with wrong signature must not forward.
	out.mu.Lock()
	out.sent = nil
	out.mu.Unlock()
	in.mu.Lock()
	in.sent = nil
	in.mu.Unlock()
	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHop:         bytes.Repeat([]byte{0x72}, 16),
		NextHopIface:    out,
		ReceivedIface:   in,
		RemainingHops:   1,
		TakenHops:       0,
		DestinationHash: append([]byte(nil), destHash...),
		ProofTimeout:    time.Now().Add(time.Hour),
		Timestamp:       time.Now(),
		OriginalLinkID:  linkID,
	})
	bad := append([]byte(nil), raw...)
	bad[len(bad)-1] ^= 0xFF
	badPkt := &packet.Packet{Raw: bad}
	if err := badPkt.Unpack(); err != nil {
		t.Fatal(err)
	}
	_ = tr.validateAndForwardLRProof(badPkt, out)
	if got := in.snapshot(); len(got) != 0 {
		t.Fatalf("forged LRPROOF forwarded %d packets", len(got))
	}
}

// TestExploratoryExpiredPathReadersAgree ensures HopsTo/NextHop match HasPath
// expiry rather than returning stale path rows.
func TestExploratoryExpiredPathReadersAgree(t *testing.T) {
	tr, iface := newHasPathTransport(t)
	dest := randomHash(t, 16)
	tr.UpdatePath(dest, []byte("next-hop-16b!!"), iface.Name, 3)
	backdatePath(tr, dest, time.Duration(PathfinderE)*time.Second+time.Second)
	if tr.HasPath(dest) {
		t.Fatal("HasPath should reject expired path")
	}
	if hops := tr.HopsTo(dest); hops != PathfinderM {
		t.Fatalf("HopsTo=%d want PathfinderM for expired path", hops)
	}
	if nh := tr.NextHop(dest); nh != nil {
		t.Fatalf("NextHop=%x want nil for expired path", nh)
	}
	if name := tr.NextHopInterface(dest); name != "" {
		t.Fatalf("NextHopInterface=%q want empty for expired path", name)
	}
	if n := len(tr.GetPathTable(nil)); n != 0 {
		t.Fatalf("GetPathTable len=%d want 0 for expired-only table", n)
	}
}

// TestExploratoryPlainAnnounceDropped matches Python packet_filter rejection
// of PLAIN announces.
func TestExploratoryPlainAnnounceDropped(t *testing.T) {
	withFastAnnounceForward(t)
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	t.Cleanup(func() { _ = tr.Close() })
	tr.SetIdentity(mustIdentity(t))
	in := newTrackingIface("in")
	out := newTrackingIface("out")
	_ = tr.RegisterInterface("in", in)
	_ = tr.RegisterInterface("out", out)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, dest := signedAnnounceWithContext(t, tr, id, packet.ContextNone)
	raw[0] = (raw[0] &^ packet.HeaderMaskDestinationType) | ((DestTypePlain << HeaderDestTypeShift) & HeaderDestTypeMask)
	handler := &countingAnnounceHandler{wantPathResponses: true}
	tr.RegisterAnnounceHandler(handler)
	tr.HandlePacket(raw, in)
	time.Sleep(50 * time.Millisecond)
	tr.processDelayedAnnounceJobs()
	if tr.HasPath(dest) {
		t.Fatal("PLAIN announce must not create a path")
	}
	if handler.calls.Load() != 0 {
		t.Fatalf("PLAIN announce invoked handlers %d times", handler.calls.Load())
	}
	if sentCount(out) != 0 {
		t.Fatalf("PLAIN announce rebroadcast count=%d", sentCount(out))
	}
}

// TestExploratoryReverseProofForLocalClientWithoutTransport ensures receipt
// proofs still return to a shared-instance client when EnableTransport is off.
func TestExploratoryReverseProofForLocalClientWithoutTransport(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	t.Cleanup(func() { _ = tr.Close() })

	wan := newRelayIface("wan")
	client := newRelayIface("unix")
	client.Type = common.IFTypeUnix
	_ = tr.RegisterInterface("wan", wan)
	_ = tr.RegisterInterface("unix", client)

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
	if !tr.forwardReverseProof(proof, wan) {
		t.Fatal("expected reverse proof forward to local client")
	}
	if got := client.snapshot(); len(got) != 1 {
		t.Fatalf("local client got %d proofs want 1", len(got))
	}
}
