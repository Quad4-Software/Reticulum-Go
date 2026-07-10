// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

func announcePayload(seed byte, n int) []byte {
	if n <= 0 {
		n = 64
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = seed + byte(i)
	}
	return out
}

func cachedAnnounce(dest []byte, seed byte) *packet.Packet {
	return &packet.Packet{
		DestinationHash: append([]byte(nil), dest...),
		Data:            announcePayload(seed, 64),
		PacketType:      packet.PacketTypeAnnounce,
	}
}

func TestCacheAnnouncePacket_NilAndEmptySafe(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	dest := randomDestHash(101)
	pkt := cachedAnnounce(dest, 0x11)

	tr.cacheAnnouncePacket(nil, pkt)
	tr.cacheAnnouncePacket(dest, nil)
	tr.cacheAnnouncePacket(dest, &packet.Packet{DestinationHash: dest})
	var nilTr *Transport
	nilTr.cacheAnnouncePacket(dest, pkt)

	if got := tr.getCachedAnnouncePacket(dest); got != nil {
		t.Fatal("empty or nil inputs must not populate cache")
	}
	if got := nilTr.getCachedAnnouncePacket(dest); got != nil {
		t.Fatal("nil transport get must return nil")
	}
}

func TestCacheAnnouncePacket_StoresIndependentCopy(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	dest := randomDestHash(102)
	pkt := cachedAnnounce(dest, 0x22)
	tr.cacheAnnouncePacket(dest, pkt)

	pkt.Data[0] ^= 0xFF
	pkt.DestinationHash[0] ^= 0xFF

	got := tr.getCachedAnnouncePacket(dest)
	if got == nil {
		t.Fatal("expected cached announce")
	}
	if got == pkt {
		t.Fatal("cache must store a copy, not the caller pointer")
	}
	if bytes.Equal(got.Data, pkt.Data) {
		t.Fatal("mutating source data must not change cached payload")
	}
	if !bytes.Equal(got.DestinationHash, dest) {
		t.Fatalf("cached dest hash = %x, want %x", got.DestinationHash, dest)
	}
}

func TestQueuePathResponseAnnounce_LocalEmitsImmediately(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("local")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(103)
	tr.cacheAnnouncePacket(dest, cachedAnnounce(dest, 0x33))
	path := &common.Path{
		NextHop:     bytes.Repeat([]byte{0x01}, 16),
		Interface:   local,
		HopCount:    3,
		LastUpdated: time.Now(),
	}

	if !tr.queuePathResponseAnnounce(dest, path, local, true) {
		t.Fatal("expected queue success with cached announce")
	}
	if n := countSends(local); n != 1 {
		t.Fatalf("local client expected 1 path response send, got %d", n)
	}

	raw := local.snapshot()[0]
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack path response: %v", err)
	}
	if pkt.PacketType != packet.PacketTypeAnnounce {
		t.Fatalf("packet type = %d, want announce", pkt.PacketType)
	}
	if pkt.Context != packet.ContextPathResponse {
		t.Fatalf("context = %#x, want PATH_RESPONSE %#x", pkt.Context, packet.ContextPathResponse)
	}
	if pkt.HeaderType != packet.HeaderType2 {
		t.Fatalf("header type = %d, want HEADER_2", pkt.HeaderType)
	}
	if pkt.Hops != 3 {
		t.Fatalf("hops = %d, want 3", pkt.Hops)
	}
	if !bytes.Equal(pkt.DestinationHash, dest) {
		t.Fatalf("dest = %x, want %x", pkt.DestinationHash, dest)
	}

	tr.mutex.RLock()
	_, stillQueued := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if stillQueued {
		t.Fatal("local path response must be one-shot and leave announce table")
	}
}

func TestQueuePathResponseAnnounce_RemoteUsesGrace(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(104)
	tr.cacheAnnouncePacket(dest, cachedAnnounce(dest, 0x44))
	path := &common.Path{
		NextHop:     bytes.Repeat([]byte{0x02}, 16),
		Interface:   wan,
		HopCount:    2,
		LastUpdated: time.Now(),
	}

	before := time.Now()
	if !tr.queuePathResponseAnnounce(dest, path, wan, false) {
		t.Fatal("expected queue success")
	}
	if n := countSends(wan); n != 0 {
		t.Fatalf("remote answer must wait for grace, got %d sends", n)
	}

	tr.mutex.RLock()
	entry := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if entry == nil {
		t.Fatal("expected announce table entry during grace")
	}
	delay := entry.RetransmitTimeout.Sub(before)
	if delay < 300*time.Millisecond || delay > 500*time.Millisecond {
		t.Fatalf("grace delay = %v, want ~%v", delay, PathRequestGrace)
	}

	entry.RetransmitTimeout = time.Now().Add(-time.Millisecond)
	tr.processAnnounceTable()
	if n := countSends(wan); n != 1 {
		t.Fatalf("after grace expiry expected 1 send, got %d", n)
	}
}

func TestQueuePathResponseAnnounce_NoCacheReturnsFalse(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(105)
	path := &common.Path{Interface: wan, HopCount: 1, LastUpdated: time.Now()}

	if tr.queuePathResponseAnnounce(dest, path, wan, true) {
		t.Fatal("queue without cached announce must fail")
	}
	if n := countSends(wan); n != 0 {
		t.Fatalf("no cache must not send, got %d", n)
	}
}

func TestQueuePathResponseAnnounce_HoldsInFlightEntry(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(106)
	cached := cachedAnnounce(dest, 0x55)
	tr.cacheAnnouncePacket(dest, cached)

	prev := &PathAnnounceEntry{
		CreatedAt:         time.Now().Add(-time.Minute),
		Packet:            cached,
		AttachedInterface: wan,
		BlockRebroadcasts: false,
	}
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = prev
	tr.mutex.Unlock()

	path := &common.Path{Interface: wan, HopCount: 4, LastUpdated: time.Now()}
	if !tr.queuePathResponseAnnounce(dest, path, wan, false) {
		t.Fatal("expected queue success")
	}

	tr.mutex.RLock()
	held := tr.heldAnnounces[string(dest)]
	cur := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if held != prev {
		t.Fatal("in-flight announce must move to heldAnnounces")
	}
	if cur == nil || cur == prev {
		t.Fatal("announce table must hold the new path-response entry")
	}
}

func TestNoteAndAnswerPendingLocalPathRequest(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("pending-local")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(107)

	tr.notePendingLocalPathRequest(dest, local)
	tr.answerPendingLocalPathRequest(dest, 7)
	if n := countSends(local); n != 0 {
		t.Fatalf("answer without cache must not send, got %d", n)
	}

	tr.notePendingLocalPathRequest(dest, local)
	tr.cacheAnnouncePacket(dest, cachedAnnounce(dest, 0x66))
	tr.answerPendingLocalPathRequest(dest, 7)
	if n := countSends(local); n != 1 {
		t.Fatalf("pending local answer expected 1 send, got %d", n)
	}

	tr.mutex.RLock()
	_, stillPending := tr.pendingLocalPathReqs[string(dest)]
	tr.mutex.RUnlock()
	if stillPending {
		t.Fatal("pending local path request must be cleared after answer")
	}

	// Second answer with nothing pending is a no-op.
	tr.answerPendingLocalPathRequest(dest, 7)
	if n := countSends(local); n != 1 {
		t.Fatalf("duplicate answer must not resend, got %d", n)
	}
}

func TestProcessAnnounceTable_CleansNilAndExhaustedRetries(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}
	destNil := randomDestHash(108)
	destDone := randomDestHash(109)
	destDue := randomDestHash(110)

	tr.mutex.Lock()
	tr.announceTable[string(destNil)] = nil
	tr.announceTable[string(destDone)] = &PathAnnounceEntry{
		Retries:           LocalRebroadcastsMax,
		RetransmitTimeout: time.Now().Add(-time.Second),
		Packet:            cachedAnnounce(destDone, 0x77),
		AttachedInterface: wan,
		BlockRebroadcasts: true,
	}
	tr.announceTable[string(destDue)] = &PathAnnounceEntry{
		Retries:           0,
		RetransmitTimeout: time.Now().Add(-time.Millisecond),
		AnnounceHops:      2,
		Packet:            cachedAnnounce(destDue, 0x88),
		AttachedInterface: wan,
		BlockRebroadcasts: true,
	}
	tr.mutex.Unlock()

	tr.processAnnounceTable()

	tr.mutex.RLock()
	_, nilLeft := tr.announceTable[string(destNil)]
	_, doneLeft := tr.announceTable[string(destDone)]
	_, dueLeft := tr.announceTable[string(destDue)]
	tr.mutex.RUnlock()
	if nilLeft || doneLeft {
		t.Fatal("nil and exhausted entries must be removed")
	}
	if dueLeft {
		t.Fatal("emitted path-response entry must be consumed")
	}
	if n := countSends(wan); n != 1 {
		t.Fatalf("due entry expected 1 send, got %d", n)
	}
}

func TestEmitAnnounceTableEntry_DisabledInterfaceDropsEntry(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("disabled-wan")
	wan.Disable()
	dest := randomDestHash(111)
	entry := &PathAnnounceEntry{
		Packet:            cachedAnnounce(dest, 0x99),
		AttachedInterface: wan,
		BlockRebroadcasts: true,
		AnnounceHops:      1,
	}
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = entry
	tr.mutex.Unlock()

	tr.emitAnnounceTableEntry(string(dest), entry)
	if n := countSends(wan); n != 0 {
		t.Fatalf("disabled iface must not send, got %d", n)
	}
	tr.mutex.RLock()
	_, left := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if left {
		t.Fatal("disabled iface emit must delete announce table entry")
	}
}

func TestEmitAnnounceTableEntry_ReinsertsHeldAnnounce(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan-held")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(112)
	held := &PathAnnounceEntry{
		CreatedAt: time.Now().Add(-time.Minute),
		Packet:    cachedAnnounce(dest, 0xAA),
	}
	entry := &PathAnnounceEntry{
		Packet:            cachedAnnounce(dest, 0xAB),
		AttachedInterface: wan,
		BlockRebroadcasts: true,
		AnnounceHops:      1,
	}
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = entry
	tr.heldAnnounces[string(dest)] = held
	tr.mutex.Unlock()

	tr.emitAnnounceTableEntry(string(dest), entry)
	if n := countSends(wan); n != 1 {
		t.Fatalf("expected path response send, got %d", n)
	}
	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	_, heldLeft := tr.heldAnnounces[string(dest)]
	tr.mutex.RUnlock()
	if heldLeft {
		t.Fatal("held entry must be consumed on reinsert")
	}
	if cur != held {
		t.Fatal("held announce must be reinserted into announce table")
	}
}

func TestBuildPathResponseWire_Errors(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.mutex.Lock()
	tr.transportIdentity = nil
	tr.mutex.Unlock()

	dest := randomDestHash(113)
	entry := &PathAnnounceEntry{
		Packet:       cachedAnnounce(dest, 0xAC),
		AnnounceHops: 1,
	}
	if _, err := tr.buildPathResponseWire(entry); err == nil {
		t.Fatal("expected error without transport identity")
	}

	tr.SetIdentity(mustIdentity(t))
	if _, err := tr.buildPathResponseWire(&PathAnnounceEntry{}); err == nil {
		t.Fatal("expected error for empty packet")
	}
	if _, err := tr.buildPathResponseWire(&PathAnnounceEntry{
		Packet: &packet.Packet{Data: []byte{0x01}},
	}); err == nil {
		t.Fatal("expected error for missing destination hash")
	}
	if _, err := tr.buildPathResponseWire(&PathAnnounceEntry{
		Packet: &packet.Packet{
			DestinationHash: []byte{0x01},
			Data:            []byte{0x02},
		},
	}); err == nil {
		t.Fatal("expected error for non-truncated destination hash")
	}
}

func TestBuildPathResponseWire_NonBlockedUsesContextNone(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	dest := randomDestHash(114)
	raw, err := tr.buildPathResponseWire(&PathAnnounceEntry{
		Packet:            cachedAnnounce(dest, 0xAD),
		AnnounceHops:      2,
		BlockRebroadcasts: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatal(err)
	}
	if pkt.Context != packet.ContextNone {
		t.Fatalf("context = %#x, want NONE", pkt.Context)
	}
}

func TestBuildPathResponseWire_MTUExceeded(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	dest := randomDestHash(115)
	_, err := tr.buildPathResponseWire(&PathAnnounceEntry{
		Packet: &packet.Packet{
			DestinationHash: dest,
			Data:            bytes.Repeat([]byte{0xEE}, packet.MTU),
		},
		AnnounceHops:      1,
		BlockRebroadcasts: true,
	})
	if err == nil {
		t.Fatal("expected MTU error for oversized announce payload")
	}
}

func TestCacheAnnouncePacket_RejectsNonTruncatedHash(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	short := []byte{0x01, 0x02}
	long := bytes.Repeat([]byte{0x03}, 32)
	pkt := &packet.Packet{Data: announcePayload(0xB0, 32)}
	tr.cacheAnnouncePacket(short, pkt)
	tr.cacheAnnouncePacket(long, pkt)
	if got := tr.getCachedAnnouncePacket(short); got != nil {
		t.Fatal("short dest hash must not cache")
	}
	if got := tr.getCachedAnnouncePacket(long); got != nil {
		t.Fatal("long dest hash must not cache")
	}
}

func TestQueuePathResponseAnnounce_UsesAnnounceTableWhenCacheEmpty(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("table-src")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(116)
	src := cachedAnnounce(dest, 0xB1)
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = &PathAnnounceEntry{
		Packet:            src,
		AttachedInterface: local,
	}
	tr.mutex.Unlock()

	path := &common.Path{Interface: local, HopCount: 2, LastUpdated: time.Now()}
	if !tr.queuePathResponseAnnounce(dest, path, local, true) {
		t.Fatal("queue must use announce-table packet when cache is empty")
	}
	if n := countSends(local); n != 1 {
		t.Fatalf("expected 1 send from table-sourced queue, got %d", n)
	}
}

func TestAnswerPendingLocalPathRequest_PrefersAnnounceHops(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("hops-local")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(117)
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		Interface:   local,
		HopCount:    1,
		LastUpdated: time.Now(),
	}
	tr.mutex.Unlock()

	tr.notePendingLocalPathRequest(dest, local)
	tr.cacheAnnouncePacket(dest, cachedAnnounce(dest, 0xB2))
	tr.answerPendingLocalPathRequest(dest, 9)
	if n := countSends(local); n != 1 {
		t.Fatalf("expected pending answer send, got %d", n)
	}
}

func TestQueuePathResponseAnnounce_FillsMissingDestHash(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("fill-dest")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(118)
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = &PathAnnounceEntry{
		Packet: &packet.Packet{
			Data: announcePayload(0xB3, 48),
		},
		AttachedInterface: local,
	}
	tr.mutex.Unlock()

	path := &common.Path{Interface: local, HopCount: 1, LastUpdated: time.Now()}
	if !tr.queuePathResponseAnnounce(dest, path, local, true) {
		t.Fatal("queue must fill missing destination hash from request")
	}
	if n := countSends(local); n != 1 {
		t.Fatalf("expected send after dest-hash fill, got %d", n)
	}
}

func TestQueuePathResponseAnnounce_RawFallbackWhenUnpackFails(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("raw-fallback")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(119)
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = &PathAnnounceEntry{
		Packet: &packet.Packet{
			Raw: announcePayload(0xB4, 40),
		},
		AttachedInterface: local,
	}
	tr.mutex.Unlock()

	path := &common.Path{Interface: local, HopCount: 1, LastUpdated: time.Now()}
	if !tr.queuePathResponseAnnounce(dest, path, local, true) {
		t.Fatal("queue must accept Raw-only announce seed when Unpack fails")
	}
	if n := countSends(local); n != 1 {
		t.Fatalf("expected send from Raw fallback, got %d", n)
	}
}

func TestProcessAnnounceTable_SkipsFutureTimeout(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("future-wan")
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(120)
	tr.mutex.Lock()
	tr.announceTable[string(dest)] = &PathAnnounceEntry{
		Retries:           0,
		RetransmitTimeout: time.Now().Add(time.Hour),
		Packet:            cachedAnnounce(dest, 0xB5),
		AttachedInterface: wan,
		BlockRebroadcasts: true,
	}
	tr.mutex.Unlock()

	tr.processAnnounceTable()
	if n := countSends(wan); n != 0 {
		t.Fatalf("future timeout must not emit, got %d", n)
	}
	tr.mutex.RLock()
	_, left := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if !left {
		t.Fatal("future entry must remain queued")
	}
}

func TestNotePendingLocalPathRequest_RejectsBadInputs(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	local := newLocalClientRelayIface("bad-pending")
	dest := randomDestHash(121)

	tr.notePendingLocalPathRequest(nil, local)
	tr.notePendingLocalPathRequest(dest, nil)
	tr.notePendingLocalPathRequest([]byte{0x01}, local)
	var nilTr *Transport
	nilTr.notePendingLocalPathRequest(dest, local)

	tr.mutex.RLock()
	n := len(tr.pendingLocalPathReqs)
	tr.mutex.RUnlock()
	if n != 0 {
		t.Fatalf("bad inputs must not record pending reqs, got %d", n)
	}
}

// Regression: shared-instance known-path answer must deliver PATH_RESPONSE.
func TestRegression_KnownPathCachedAnnounceAnswersLocalClient(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("lc")
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(115)
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x10}, 16),
		Interface:   wan,
		HopCount:    4,
		LastUpdated: time.Now(),
	}
	tr.announcePacketCache[string(dest)] = cachedAnnounce(dest, 0xAE)
	tr.mutex.Unlock()

	tr.processPathRequest(dest, local, nil, bytes.Repeat([]byte{0x01}, 16))
	if n := countSends(local); n != 1 {
		t.Fatalf("local client must receive path response, got %d", n)
	}
	if n := countSends(wan); n != 0 {
		t.Fatalf("known-path answer must not rediscover on wan, got %d", n)
	}
}

// Regression: known path without cache must fall through to local-client forward.
func TestRegression_KnownPathWithoutCacheForwardsLocalClientPR(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("lc2")
	wan := newRelayIface("wan2")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(116)
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x11}, 16),
		Interface:   wan,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, local, nil, bytes.Repeat([]byte{0x02}, 16))
	if n := countSends(wan); n < 1 {
		t.Fatalf("local-client PR without cache must forward discovery, got %d", n)
	}
	tr.mutex.RLock()
	_, pending := tr.pendingLocalPathReqs[string(dest)]
	tr.mutex.RUnlock()
	if !pending {
		t.Fatal("forwarded local-client PR must record pendingLocalPathReqs")
	}
}

// Regression: unregistering a local client must drop its pending path requests.
func TestRegression_UnregisterClearsPendingLocalPathRequests(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	local := newLocalClientRelayIface("lc3")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	dest := randomDestHash(117)
	tr.notePendingLocalPathRequest(dest, local)
	tr.UnregisterInterface(local.GetName())

	tr.mutex.RLock()
	_, pending := tr.pendingLocalPathReqs[string(dest)]
	tr.mutex.RUnlock()
	if pending {
		t.Fatal("pending local path request must be cleared on unregister")
	}
}

func TestAnnounceTable_ConcurrentCacheQueueProcess(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	local := newLocalClientRelayIface("race-local")
	wan := newRelayIface("race-wan")
	if err := tr.RegisterInterface(local.GetName(), local); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface(wan.GetName(), wan); err != nil {
		t.Fatal(err)
	}

	const n = 32
	dests := make([][]byte, n)
	for i := range dests {
		dests[i] = randomDestHash(200 + i)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(3)
		dest := dests[i]
		go func() {
			defer wg.Done()
			tr.cacheAnnouncePacket(dest, cachedAnnounce(dest, byte(i)))
		}()
		go func() {
			defer wg.Done()
			path := &common.Path{
				NextHop:     bytes.Repeat([]byte{0x20}, 16),
				Interface:   wan,
				HopCount:    1,
				LastUpdated: time.Now(),
			}
			_ = tr.queuePathResponseAnnounce(dest, path, local, true)
		}()
		go func() {
			defer wg.Done()
			tr.notePendingLocalPathRequest(dest, local)
			tr.answerPendingLocalPathRequest(dest, 1)
			tr.processAnnounceTable()
		}()
	}
	wg.Wait()
}
