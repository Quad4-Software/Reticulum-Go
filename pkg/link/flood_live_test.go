// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

// craftLinkRequest builds a wire-format LINKREQUEST for destHash with a
// random ephemeral keypair payload so every packet produces a distinct
// linkID and forces the responder's full handshake work (keygen, ECDH,
// Ed25519 sign, proof transmit).
func craftLinkRequest(t *testing.T, destHash []byte) []byte {
	t.Helper()
	data := make([]byte, ECPubSize+LinkMTUSize)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	// Valid signalling bytes: mode=AES256CBC, mtu=500.
	copy(data[ECPubSize:], signallingBytes(500, ModeAES256CBC))
	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		DestinationType: destination.Single,
		DestinationHash: append([]byte(nil), destHash...),
		Data:            data,
	}
	if err := pkt.Pack(); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return pkt.Raw
}

// pipeNode wires a Transport plus PipeInterface pair; pair() connects two
// nodes so packets flow between them through the real inbound path.
type pipeNode struct {
	tr   *transport.Transport
	pipe *PipeInterface
}

func newPipeNode(t *testing.T, name string) *pipeNode {
	t.Helper()
	tr := transport.NewTransport(&common.ReticulumConfig{})
	p := NewPipeInterface(name)
	n := &pipeNode{tr: tr, pipe: p}
	p.tr = tr
	if err := tr.RegisterInterface(name, p); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	return n
}

func pair(a, b *pipeNode) {
	a.pipe.peer = b.pipe
	b.pipe.peer = a.pipe
}

// establishLink announces destA on nodeA and returns the initiator link
// (on nodeB) plus the responder link (on nodeA) once active.
func establishLink(t *testing.T, nodeA, nodeB *pipeNode, destA *destination.Destination) (initiator, responder *Link) {
	t.Helper()

	var respMu sync.Mutex
	destA.SetLinkEstablishedCallback(func(l any) {
		if lk, ok := l.(*Link); ok {
			respMu.Lock()
			responder = lk
			respMu.Unlock()
		}
	})
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !nodeB.tr.HasPath(destA.GetHash()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !nodeB.tr.HasPath(destA.GetHash()) {
		t.Fatal("nodeB never learned path to destA")
	}

	estCh := make(chan *Link, 1)
	lkB := NewLink(destA, nodeB.tr, nodeB.pipe, func(l *Link) { estCh <- l }, nil)
	if err := lkB.Establish(); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	select {
	case initiator = <-estCh:
	case <-time.After(10 * time.Second):
		t.Fatal("link never established")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		respMu.Lock()
		have := responder != nil
		respMu.Unlock()
		if have {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if responder == nil {
		t.Fatal("responder link never became active")
	}
	return initiator, responder
}

// TestFloodLinkRequestCap drives N unauthenticated LINKREQUESTs through
// the real inbound path and asserts the registered-link cap holds.
func TestFloodLinkRequestCap(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "floodA")
	nodeB := newPipeNode(t, "floodB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "floodapp", nodeA.tr, "svc")
	if err != nil {
		t.Fatal(err)
	}
	destA.AcceptsLinks(true)

	const N = 6000
	g0 := runtime.NumGoroutine()
	for i := 0; i < N; i++ {
		nodeA.tr.HandlePacket(craftLinkRequest(t, destA.GetHash()), nodeA.pipe)
	}

	// Wait for handler drain: link table stops changing.
	var links int
	prevG, stable := -1, 0
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && stable < 4 {
		time.Sleep(150 * time.Millisecond)
		g := runtime.NumGoroutine()
		links = nodeA.tr.LinkCount()
		if g == prevG {
			stable++
		} else {
			stable = 0
		}
		prevG = g
	}

	proofFrames := nodeA.pipe.delivered.Load()
	t.Logf("LINKREQUEST flood: %d sent -> links=%d proof_frames=%d goroutines=%d(+%d)",
		N, links, proofFrames, runtime.NumGoroutine(), runtime.NumGoroutine()-g0)

	if links > transport.MaxRegisteredLinks {
		t.Fatalf("link cap breached: %d > %d", links, transport.MaxRegisteredLinks)
	}
	if got := runtime.NumGoroutine() - g0; got > transport.MaxRegisteredLinks+transport.MaxConcurrentPacketHandlers+64 {
		t.Fatalf("goroutine growth unbounded: +%d for %d requests", got, N)
	}
	t.Logf("PASS: %d LINKREQUESTs -> %d links (cap %d)", N, links, transport.MaxRegisteredLinks)
}

// TestFloodLinkRequestReplayDedup replays one identical LINKREQUEST N
// times. The first costs a full handshake; replays must be answered by
// cached-proof resend - one link, no repeated crypto.
func TestFloodLinkRequestReplayDedup(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "replayA")
	nodeB := newPipeNode(t, "replayB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "floodapp", nodeA.tr, "svc")
	if err != nil {
		t.Fatal(err)
	}
	destA.AcceptsLinks(true)

	// Wire-level identical replays are dropped by the transport packet
	// filter before reaching the handler; the linkID dedup covers replays
	// that re-arrive after hashlist eviction, plus in-process callers.
	// Exercise the handler path directly to prove it.
	raw := craftLinkRequest(t, destA.GetHash())
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("Unpack: %v", err)
	}

	start := time.Now()
	l1, err := HandleIncomingLinkRequest(pkt, destA, nodeA.tr, nodeA.pipe)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if l1 == nil {
		t.Fatal("first request returned nil link")
	}
	firstElapsed := time.Since(start)

	const N = 2000
	replayStart := time.Now()
	var last *Link
	for i := 0; i < N; i++ {
		l, err := HandleIncomingLinkRequest(pkt, destA, nodeA.tr, nodeA.pipe)
		if err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
		last = l
	}
	replayElapsed := time.Since(replayStart)

	links := nodeA.tr.LinkCount()
	proofs := nodeA.pipe.delivered.Load()
	t.Logf("LR replay: 1 + %d identical requests -> links=%d proof_frames=%d first=%s replay=%s/pkt",
		N, links, proofs, firstElapsed, replayElapsed/N)

	if links != 1 {
		t.Fatalf("replay produced %d links, want exactly 1", links)
	}
	if last == nil {
		t.Fatal("replay returned nil link")
	}
	// The first request pays keygen+ECDH+sign (~ms); each replay is a
	// FindLink + cached-proof resend and must be orders cheaper.
	if replayElapsed/N > firstElapsed/20 {
		t.Fatalf("replay not deduplicated: first=%s replay=%s/pkt", firstElapsed, replayElapsed/N)
	}
	t.Logf("PASS: %d replays -> 1 link, proofs resent at %s/pkt (first cost %s)",
		N, replayElapsed/N, firstElapsed)
}

// TestFloodResourceReqAmplification proves the per-link REQ limiter caps
// the small-request/large-response amplification: a REQ storm must not
// dispatch unbounded part retransmissions.
func TestFloodResourceReqAmplification(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "reqA")
	nodeB := newPipeNode(t, "reqB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "floodapp", nodeA.tr, "svc")
	if err != nil {
		t.Fatal(err)
	}
	destA.AcceptsLinks(true)

	initiator, responder := establishLink(t, nodeA, nodeB, destA)

	// Give the responder a real outbound resource so REQ dispatch finds
	// matching map hashes and actually retransmits parts. outgoingRes is
	// set directly rather than via SendResource so the initiator's default
	// AcceptNone rejection cannot clear it mid-test.
	payload := make([]byte, 64*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	res, err := resource.New(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	sdu := responder.resourceSDU()
	if err := res.PrepareOutboundForLink(responder.encrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}
	res.Activate()
	responder.outgoingMu.Lock()
	responder.outgoingRes = res
	responder.outgoingMu.Unlock()
	responder.mutex.RLock()
	mdu := responder.mdu
	responder.mutex.RUnlock()

	// REQ body: 0x00 + resourceHash + map hashes selected from the
	// sender's own hashmap so parts really dispatch.
	resHash := res.GetHash()
	hm := res.HashmapSegment(mdu, 0)
	var reqHashes []byte
	for i := 0; i+resource.MapHashLen <= len(hm) && len(reqHashes) < 20*resource.MapHashLen; i += resource.MapHashLen {
		reqHashes = append(reqHashes, hm[i:i+resource.MapHashLen]...)
	}
	if len(reqHashes) == 0 {
		t.Fatal("no map hashes in resource hashmap segment")
	}
	reqBody := append(append([]byte{0x00}, resHash...), reqHashes...)

	health.Default.Reset()
	before := nodeA.pipe.delivered.Load()

	const N = 3000
	for i := 0; i < N; i++ {
		if err := initiator.SendPacketWithContext(reqBody, packet.ContextResourceReq); err != nil {
			t.Fatalf("REQ send %d: %v", i, err)
		}
	}

	// Let the storm drain.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap := health.Default.SnapshotTransport()
		if snap.ResourceReqDrop.Total > 0 || len(responder.pendingRequests) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)

	after := nodeA.pipe.delivered.Load()
	dropped := health.Default.SnapshotTransport().ResourceReqDrop.Total
	partFrames := after - before
	// Burst 384 + refill 128/s over the storm; each admitted REQ can emit
	// at most the requested hash count (<=20) part frames.
	maxFrames := int64(resourceReqBurst+resourceReqRate*5) * int64(len(reqHashes)/resource.MapHashLen) * 2
	t.Logf("REQ storm: %d reqs -> %d part frames outbound, %d reqs dropped by limiter",
		N, partFrames, dropped)

	if dropped == 0 {
		t.Fatal("REQ limiter never tripped under 3000-packet storm")
	}
	if partFrames > maxFrames {
		t.Fatalf("REQ amplification unbounded: %d frames > %d", partFrames, maxFrames)
	}
	t.Logf("PASS: %d REQs -> %d part frames (limiter held, %d dropped)", N, partFrames, dropped)
}

// TestFloodResourceAdvSupersede floods valid resource advertisements on an
// established link. Each new adv must supersede the previous one so at
// most one incoming assembly (and watchdog) lives per link.
func TestFloodResourceAdvSupersede(t *testing.T) {
	skipHeavyLinkTestsIfShort(t)

	nodeA := newPipeNode(t, "advA")
	nodeB := newPipeNode(t, "advB")
	defer nodeA.tr.Close()
	defer nodeB.tr.Close()
	pair(nodeA, nodeB)

	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "floodapp", nodeA.tr, "svc")
	if err != nil {
		t.Fatal(err)
	}
	destA.AcceptsLinks(true)
	if err := destA.RegisterRequestHandlerAny("echo",
		func(path string, data []byte, requestID []byte, linkID []byte, remote *identity.Identity, at int64) any {
			return []byte("ok")
		}, destination.AllowAll, nil); err != nil {
		t.Fatalf("RegisterRequestHandlerAny: %v", err)
	}

	initiator, responder := establishLink(t, nodeA, nodeB, destA)
	responder.mutex.RLock()
	mdu := responder.mdu
	responder.mutex.RUnlock()

	sdu := responder.resourceSDU()
	if sdu <= 0 {
		t.Fatal("responder sdu <= 0")
	}

	// Craft advs with distinct hashes; each supersedes the previous.
	mkAdv := func() []byte {
		hash := make([]byte, 32)
		rh := make([]byte, resource.RandomHashSize)
		reqID := make([]byte, 16)
		hm := make([]byte, 4*resource.MapHashLen)
		rand.Read(hash)
		rand.Read(rh)
		rand.Read(reqID)
		rand.Read(hm)
		adv := &resource.ResourceAdvertisement{
			TransferSize:  int64(4 * sdu),
			DataSize:      int64(4 * sdu),
			Parts:         4,
			Hash:          hash,
			RandomHash:    rh,
			OriginalHash:  hash,
			Hashmap:       hm,
			RequestID:     reqID,
			Flags:         resource.AdvFlagIsRequest,
			SegmentIndex:  1,
			TotalSegments: 1,
		}
		packed, err := adv.Pack(0, mdu)
		if err != nil {
			t.Fatalf("adv pack: %v", err)
		}
		return packed
	}

	const N = 800
	g0 := runtime.NumGoroutine()
	for i := 0; i < N; i++ {
		if err := initiator.SendPacketWithContext(mkAdv(), packet.ContextResourceAdv); err != nil {
			t.Fatalf("adv send %d: %v", i, err)
		}
	}

	// Wait for inbound processing to settle.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		responder.incomingMu.Lock()
		rx := responder.incomingRx
		responder.incomingMu.Unlock()
		if rx != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)

	responder.incomingMu.Lock()
	rx := responder.incomingRx
	responder.incomingMu.Unlock()
	g1 := runtime.NumGoroutine()

	t.Logf("adv flood: %d sent -> incomingRx=%v goroutines=%d(+%d)", N, rx != nil, g1, g1-g0)
	if rx == nil {
		t.Fatal("no incoming resource active after flood")
	}
	// One incoming asm per link + watchdog; superseded ones exit.
	if g1-g0 > 64 {
		t.Fatalf("supersede leaked goroutines: +%d for %d advs", g1-g0, N)
	}
	t.Logf("PASS: %d advs -> single incoming assembly, +%d goroutines", N, g1-g0)
}
