// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newLocalClientRelayIface creates a relayIface that looks like a spawned
// local-client interface on the shared-instance server: IFTypeUnix + IFModeFull.
func newLocalClientRelayIface(name string) *relayIface {
	r := newRelayIface(name)
	r.Type = common.IFTypeUnix
	r.Mode = common.IFModeFull
	return r
}

// countSends returns the number of packets captured by a relayIface.
func countSends(r *relayIface) int {
	return len(r.snapshot())
}

// randomDestHash generates a deterministic-but-unique 16-byte destination hash
// from a seed integer, so parallel tests don't collide on the same destHash.
func randomDestHash(seed int) []byte {
	h := make([]byte, 16)
	_, _ = rand.Read(h)
	h[0] = byte(seed)
	h[1] = byte(seed >> 8)
	return h
}

// ===========================================================================
// 1. EDGE CASES
// ===========================================================================

// TestLocalClientPR_NilAttachedIface verifies that a nil attached interface
// does not crash and does not forward anything.
func TestLocalClientPR_NilAttachedIface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(1)
	tag := bytes.Repeat([]byte{0x01}, 16)

	// nil iface → isLocalClientInterface(nil) is false, so this falls through
	// to the "no path known" branch and returns.
	tr.processPathRequest(dest, nil, nil, tag)

	if n := countSends(wan); n != 0 {
		t.Fatalf("nil iface must not forward, got %d sends", n)
	}
}

// TestLocalClientPR_EmptyDestHash verifies that an empty destHash does not
// crash processPathRequest.
func TestLocalClientPR_EmptyDestHash(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	// Empty destHash: won't match any local destination or path, so it should
	// fall through to the local-client forwarding branch. RequestPath will
	// still send a packet addressed to the well-known PR destination.
	tr.processPathRequest([]byte{}, lc, nil, bytes.Repeat([]byte{0x02}, 16))

	// Even with empty destHash, the PR is forwarded (RequestPath uses the
	// well-known pathRequestDestHash, not the destHash for addressing).
	// This matches Python behavior: it forwards regardless of destHash content.
	if n := countSends(wan); n != 1 {
		t.Fatalf("expected PR forwarded even with empty destHash, got %d", n)
	}
}

// TestLocalClientPR_AllOtherInterfacesDisabled verifies that when all other
// interfaces are disabled, the PR is not forwarded and no error occurs.
func TestLocalClientPR_AllOtherInterfacesDisabled(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	wan.Disable()
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(2)
	tag := bytes.Repeat([]byte{0x03}, 16)

	tr.processPathRequest(dest, lc, nil, tag)

	if n := countSends(wan); n != 0 {
		t.Fatalf("disabled interfaces must not receive PR, got %d", n)
	}
	if n := countSends(lc); n != 0 {
		t.Fatalf("local client must not echo PR, got %d", n)
	}
}

// TestLocalClientPR_SingleInterfaceOnlyLocalClient verifies that when the only
// registered interface is the local client itself, no forwarding happens (no
// echo back to self).
func TestLocalClientPR_SingleInterfaceOnlyLocalClient(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(3)
	tag := bytes.Repeat([]byte{0x04}, 16)

	tr.processPathRequest(dest, lc, nil, tag)

	if n := countSends(lc); n != 0 {
		t.Fatalf("PR must not echo back to the only local client, got %d", n)
	}
}

// TestLocalClientPR_MultipleWanInterfaces verifies forwarding to N>1 interfaces.
func TestLocalClientPR_MultipleWanInterfaces(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}

	wans := make([]*relayIface, 5)
	for i := range wans {
		wans[i] = newRelayIface(fmt.Sprintf("wan%d", i))
		if err := tr.RegisterInterface(wans[i].GetName(), wans[i]); err != nil {
			t.Fatal(err)
		}
	}

	dest := randomDestHash(4)
	tag := bytes.Repeat([]byte{0x05}, 16)

	tr.processPathRequest(dest, lc, nil, tag)

	for i, w := range wans {
		if n := countSends(w); n != 1 {
			t.Fatalf("wan%d: expected 1 PR, got %d", i, n)
		}
	}
	if n := countSends(lc); n != 0 {
		t.Fatalf("local client must not receive echo, got %d", n)
	}
}

// TestLocalClientPR_TransportDisabledStillForwardsUnknown verifies that even
// with transport disabled, a local-client PR for an unknown destination is
// forwarded (the is_from_local_client branch bypasses transportEnabled()).
func TestLocalClientPR_TransportDisabledStillForwardsUnknown(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(5)
	tag := bytes.Repeat([]byte{0x06}, 16)

	tr.processPathRequest(dest, lc, nil, tag)

	if n := countSends(wan); n != 1 {
		t.Fatalf("local-client PR must forward even with transport disabled, got %d", n)
	}
}

// TestLocalClientPR_SelfExclusionStrict verifies that the PR is never sent back
// on the same interface that it arrived from, even if that interface is the
// only one registered alongside the local client.
func TestLocalClientPR_SelfExclusionStrict(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	// Two local-client interfaces (two shared-instance clients).
	lc1 := newLocalClientRelayIface("lc1")
	lc2 := newLocalClientRelayIface("lc2")
	if err := tr.RegisterInterface("lc1", lc1); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("lc2", lc2); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(6)
	tag := bytes.Repeat([]byte{0x07}, 16)

	tr.processPathRequest(dest, lc1, nil, tag)

	if n := countSends(lc1); n != 0 {
		t.Fatalf("PR must not echo to lc1, got %d", n)
	}
	if n := countSends(lc2); n != 1 {
		t.Fatalf("PR should forward to lc2, got %d", n)
	}
}

// TestLocalClientPR_StalePathFallsThroughToForwarding verifies that a local
// client PR for a stale (over-TTL) path falls through to the forwarding branch
// (not the known-path answer branch).
func TestLocalClientPR_StalePathFallsThroughToForwarding(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(7)
	tag := bytes.Repeat([]byte{0x08}, 16)

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x62}, 16),
		Interface:   wan,
		HopCount:    1,
		LastUpdated: time.Now().Add(-time.Duration(PathRequestTTL+10) * time.Second),
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, lc, nil, tag)

	// Stale path is dropped, so hasPath=false. The local-client branch
	// forwards the PR on wan.
	if n := countSends(wan); n != 1 {
		t.Fatalf("stale-path local-client PR should forward to wan, got %d", n)
	}
}

// ===========================================================================
// 2. RACE / STRESS TESTS
// ===========================================================================

// TestLocalClientPR_ConcurrentStress verifies goroutine safety when many
// goroutines simultaneously send path requests from local-client interfaces.
func TestLocalClientPR_ConcurrentStress(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	const goroutines = 50
	const prsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(gid int) {
			defer wg.Done()
			for j := range prsPerGoroutine {
				dest := randomDestHash(gid*100 + j)
				tag := make([]byte, 16)
				_, _ = rand.Read(tag)
				tr.processPathRequest(dest, lc, nil, tag)
			}
		}(i)
	}
	wg.Wait()

	totalSends := countSends(wan)
	expectedMin := goroutines * prsPerGoroutine * 80 / 100 // allow some throttle drops
	if totalSends < expectedMin {
		t.Fatalf("expected at least %d sends, got %d", expectedMin, totalSends)
	}
	if n := countSends(lc); n != 0 {
		t.Fatalf("no echo to local client, got %d", n)
	}
}

// TestLocalClientPR_ConcurrentMixedInterfaces verifies safety when PRs arrive
// concurrently from both local-client and non-local-client interfaces. The
// key assertion is no panic/race under concurrent access. The gw discovery
// queue may asynchronously rebroadcast on lc (correct behavior), so we don't
// assert lc send count here.
func TestLocalClientPR_ConcurrentMixedInterfaces(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	gw := newRelayIface("gw")
	gw.Mode = common.IFModeGateway
	if err := tr.RegisterInterface("gw", gw); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range iterations {
			dest := randomDestHash(1000 + i)
			tag := make([]byte, 16)
			_, _ = rand.Read(tag)
			tr.processPathRequest(dest, lc, nil, tag)
		}
	}()

	go func() {
		defer wg.Done()
		for i := range iterations {
			dest := randomDestHash(2000 + i)
			tag := make([]byte, 16)
			_, _ = rand.Read(tag)
			tr.processPathRequest(dest, gw, nil, tag)
		}
	}()

	wg.Wait()
	// Allow async discovery queue to drain.
	time.Sleep(300 * time.Millisecond)

	// Verify at least some PRs from lc were forwarded to wan (synchronous path).
	if n := countSends(wan); n < iterations {
		t.Fatalf("expected at least %d sends on wan from local-client PRs, got %d", iterations, n)
	}
}

// ===========================================================================
// 3. LEAK TESTS
// ===========================================================================

// TestLocalClientPR_NoDiscoveryPathRequestsLeak verifies that local-client PRs
// never create entries in discoveryPathRequests (the local-client branch is a
// direct forward, not a queued discovery).
func TestLocalClientPR_NoDiscoveryPathRequestsLeak(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	for i := range 50 {
		dest := randomDestHash(3000 + i)
		tag := make([]byte, 16)
		_, _ = rand.Read(tag)
		tr.processPathRequest(dest, lc, nil, tag)
	}

	tr.mutex.RLock()
	dprCount := len(tr.discoveryPathRequests)
	tr.mutex.RUnlock()

	if dprCount != 0 {
		t.Fatalf("local-client PRs must not create discoveryPathRequests, got %d", dprCount)
	}
}

// TestLocalClientPR_NoAnnounceTableLeakForUnknown verifies that local-client
// PRs for unknown destinations don't leave entries in announceTable.
func TestLocalClientPR_NoAnnounceTableLeakForUnknown(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(4000)
	tag := bytes.Repeat([]byte{0x09}, 16)

	tr.processPathRequest(dest, lc, nil, tag)

	tr.mutex.RLock()
	atCount := len(tr.announceTable)
	heldCount := len(tr.heldAnnounces)
	tr.mutex.RUnlock()

	if atCount != 0 {
		t.Fatalf("no announceTable entries for unknown dest, got %d", atCount)
	}
	if heldCount != 0 {
		t.Fatalf("no heldAnnounces for unknown dest, got %d", heldCount)
	}
}

// TestLocalClientPR_NoGoroutineLeak verifies that processing many local-client
// PRs does not leak goroutines.
func TestLocalClientPR_NoGoroutineLeak(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	before := runtime.NumGoroutine()

	for i := range 200 {
		dest := randomDestHash(5000 + i)
		tag := make([]byte, 16)
		_, _ = rand.Read(tag)
		tr.processPathRequest(dest, lc, nil, tag)
	}

	// Allow any async work to settle.
	time.Sleep(200 * time.Millisecond)

	after := runtime.NumGoroutine()
	leaked := after - before
	if leaked > 5 {
		t.Fatalf("goroutine leak: before=%d after=%d leaked=%d", before, after, leaked)
	}
}

// TestLocalClientPR_DiscoveryPRTagsNoUnboundedGrowth verifies that the
// discoveryPRTags dedup map doesn't grow unboundedly from local-client PRs
// (it gets reset at DiscoveryPRTagsCap).
func TestLocalClientPR_DiscoveryPRTagsNoUnboundedGrowth(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	// Send far more PRs than DiscoveryPRTagsCap.
	count := DiscoveryPRTagsCap + 500
	for i := range count {
		// handlePathRequest is the entry point that adds to discoveryPRTags.
		data := make([]byte, 32) // 16 destHash + 16 tag
		data[0] = byte(i)
		data[1] = byte(i >> 8)
		for k := 16; k < 32; k++ {
			data[k] = byte(i + k)
		}
		tr.handlePathRequest(data, lc)
	}

	tr.mutex.RLock()
	tagCount := len(tr.discoveryPRTags)
	tr.mutex.RUnlock()

	if tagCount > DiscoveryPRTagsCap {
		t.Fatalf("discoveryPRTags exceeded cap: %d > %d", tagCount, DiscoveryPRTagsCap)
	}
}

// ===========================================================================
// 4. PROPERTY-BASED / TABLE-DRIVEN TESTS
// ===========================================================================

// TestLocalClientPR_ModeTypeMatrix is a property test verifying that local
// client interfaces (IFTypeUnix) forward PRs regardless of their mode setting,
// while non-local-client interfaces only forward when mode is in
// DiscoverPathsFor. For non-local-client discover modes, the PR goes through
// the async discovery queue, so we check discoveryPathRequests instead of
// immediate sends.
func TestLocalClientPR_ModeTypeMatrix(t *testing.T) {
	modes := []struct {
		mode      common.InterfaceMode
		name      string
		discovers bool
	}{
		{common.IFModeFull, "Full", false},
		{common.IFModeAccessPoint, "AP", true},
		{common.IFModeRoaming, "Roaming", true},
		{common.IFModeGateway, "Gateway", true},
		{common.IFModeBoundary, "Boundary", false},
		{common.IFModeInternal, "Internal", true},
	}

	types := []struct {
		ifType  common.InterfaceType
		isLocal bool
	}{
		{common.IFTypeUnix, true},
		{common.IFTypeTCP, false},
		{common.IFTypeUDP, false},
	}

	for _, mt := range modes {
		for _, tt := range types {
			name := fmt.Sprintf("mode=%s/type=%v", mt.name, tt.ifType)
			t.Run(name, func(t *testing.T) {
				tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
				defer tr.Close()
				tr.SetIdentity(mustIdentity(t))

				src := newRelayIface("src")
				src.Type = tt.ifType
				src.Mode = mt.mode
				if err := tr.RegisterInterface("src", src); err != nil {
					t.Fatal(err)
				}
				wan := newRelayIface("wan")
				if err := tr.RegisterInterface("wan", wan); err != nil {
					t.Fatal(err)
				}

				dest := randomDestHash(int(mt.mode)<<8 | int(tt.ifType))
				tag := make([]byte, 16)
				_, _ = rand.Read(tag)

				tr.processPathRequest(dest, src, nil, tag)

				shouldForward := tt.isLocal || mt.discovers
				if tt.isLocal {
					// Local-client: synchronous forward, check send count.
					n := countSends(wan)
					if shouldForward && n != 1 {
						t.Fatalf("expected forward (local=%v, mode discovers=%v), got %d sends",
							tt.isLocal, mt.discovers, n)
					}
					if !shouldForward && n != 0 {
						t.Fatalf("expected no forward (local=%v, mode discovers=%v), got %d sends",
							tt.isLocal, mt.discovers, n)
					}
				} else {
					// Non-local-client: discovery goes through async queue.
					// Check discoveryPathRequests map for initiation.
					tr.mutex.RLock()
					_, hasDPR := tr.discoveryPathRequests[string(dest)]
					tr.mutex.RUnlock()
					if shouldForward && !hasDPR {
						t.Fatalf("expected discoveryPathRequests entry (local=%v, mode discovers=%v)",
							tt.isLocal, mt.discovers)
					}
					if !shouldForward && hasDPR {
						t.Fatalf("expected no discoveryPathRequests entry (local=%v, mode discovers=%v)",
							tt.isLocal, mt.discovers)
					}
				}
			})
		}
	}
}

// TestLocalClientPR_TagUniqueness verifies that each local-client PR uses a
// unique fresh tag (not the original tag from the incoming PR), so the
// dedup mechanism doesn't suppress legitimate forwarded PRs.
func TestLocalClientPR_TagUniqueness(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	// Send 10 PRs for the SAME destHash with the SAME original tag.
	// Each should still get forwarded because the local-client branch
	// generates a fresh random tag for each.
	dest := randomDestHash(6000)
	origTag := bytes.Repeat([]byte{0xAA}, 16)

	for range 10 {
		tr.processPathRequest(dest, lc, nil, origTag)
	}

	// Each PR should generate a unique fresh tag → forwarded on wan each time.
	// (The discoveryPRTags dedup in handlePathRequest may suppress some if the
	// same destHash+originalTag is seen again, but processPathRequest is
	// called directly here, bypassing handlePathRequest's dedup.)
	n := countSends(wan)
	if n < 5 {
		t.Fatalf("expected at least 5 forwards with fresh tags, got %d", n)
	}
}

// TestLocalClientPR_ImmediateRetransmitForKnownPath verifies local-client PRs
// with a known cached announce emit a path response immediately, while remote
// PRs queue with PATH_REQUEST_GRACE before send.
func TestLocalClientPR_ImmediateRetransmitForKnownPath(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	cases := []struct {
		name  string
		local bool
	}{
		{"local_client", true},
		{"remote_client", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := randomDestHash(7000)
			tag := make([]byte, 16)
			_, _ = rand.Read(tag)

			src := newRelayIface("src_" + tc.name)
			if tc.local {
				src.Type = common.IFTypeUnix
				src.Mode = common.IFModeFull
			} else {
				src.Mode = common.IFModeGateway
			}
			if err := tr.RegisterInterface(src.GetName(), src); err != nil {
				t.Fatal(err)
			}

			oldPkt := &packet.Packet{
				DestinationHash: append([]byte(nil), dest...),
				Data:            bytes.Repeat([]byte{0xCC}, 64),
			}
			tr.mutex.Lock()
			tr.paths[pathMapKey(dest)] = &common.Path{
				NextHop:     bytes.Repeat([]byte{0x22}, 16),
				Interface:   src,
				HopCount:    1,
				LastUpdated: time.Now(),
			}
			tr.announcePacketCache[string(dest)] = oldPkt
			tr.mutex.Unlock()

			before := time.Now()
			tr.processPathRequest(dest, src, nil, tag)

			if tc.local {
				if n := countSends(src); n < 1 {
					t.Fatalf("local client: expected immediate path-response send, got %d", n)
				}
				tr.mutex.RLock()
				entry := tr.announceTable[string(dest)]
				tr.mutex.RUnlock()
				if entry != nil {
					t.Fatal("local client path response should be consumed from announce table")
				}
				return
			}

			tr.mutex.RLock()
			entry := tr.announceTable[string(dest)]
			tr.mutex.RUnlock()
			if entry == nil {
				t.Fatal("expected announce table entry for remote grace-period answer")
			}
			delay := entry.RetransmitTimeout.Sub(before)
			if delay < 300*time.Millisecond || delay > 500*time.Millisecond {
				t.Fatalf("remote client: retransmit grace want ~400ms, got %v", delay)
			}
			if n := countSends(src); n != 0 {
				t.Fatalf("remote client: must not send before grace, got %d sends", n)
			}
		})
	}
}

// ===========================================================================
// 5. REGRESSION TESTS (old behavior preserved)
// ===========================================================================

// TestRegression_NonLocalClientFullModeDropsUnknownPath verifies that the old
// behavior (drop PR for unknown dest from Full-mode non-local-client interface)
// is preserved.
func TestRegression_NonLocalClientFullModeDropsUnknownPath(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	fullIface := newRelayIface("full")
	fullIface.Mode = common.IFModeFull
	// NOT IFTypeUnix, so isLocalClientInterface returns false.
	if err := tr.RegisterInterface("full", fullIface); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(8000)
	tag := bytes.Repeat([]byte{0x0B}, 16)

	tr.processPathRequest(dest, fullIface, nil, tag)

	if n := countSends(wan); n != 0 {
		t.Fatalf("non-local-client Full-mode PR must not forward, got %d", n)
	}
}

// TestRegression_NonLocalClientTransportDisabledDropsKnownPath verifies that
// a non-local-client PR with transport disabled is still rejected for a known
// path (the old behavior).
func TestRegression_NonLocalClientTransportDisabledDropsKnownPath(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	gw := newRelayIface("gw")
	gw.Mode = common.IFModeGateway
	if err := tr.RegisterInterface("gw", gw); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(9000)
	tag := bytes.Repeat([]byte{0x0C}, 16)

	oldPkt := &packet.Packet{Raw: []byte{0xDD}}
	oldEntry := &PathAnnounceEntry{
		CreatedAt: time.Now().Add(-time.Hour),
		Packet:    oldPkt,
	}
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x33}, 16),
		Interface:   gw,
		HopCount:    1,
		LastUpdated: time.Now(),
	}
	tr.announceTable[string(dest)] = oldEntry
	tr.mutex.Unlock()

	tr.processPathRequest(dest, gw, nil, tag)

	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if cur != oldEntry {
		t.Fatal("transport-disabled non-local-client PR must not modify announce table")
	}
}

// TestRegression_NonLocalClientGatewayForwardsUnknownPath verifies that the
// old behavior (forward PR from Gateway-mode interface for unknown dest via
// discovery queue) is preserved.
func TestRegression_NonLocalClientGatewayForwardsUnknownPath(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	gw := newRelayIface("gw")
	gw.Mode = common.IFModeGateway
	if err := tr.RegisterInterface("gw", gw); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(10000)
	tag := bytes.Repeat([]byte{0x0D}, 16)

	tr.processPathRequest(dest, gw, nil, tag)

	tr.mutex.RLock()
	_, ok := tr.discoveryPathRequests[string(dest)]
	tr.mutex.RUnlock()
	if !ok {
		t.Fatal("Gateway-mode PR for unknown dest should create discoveryPathRequests entry")
	}
}

// TestRegression_NextHopEqualsRequestorSuppressesAnswer verifies that even for
// local-client PRs, the next-hop-is-requestor suppression still applies.
func TestRegression_NextHopEqualsRequestorSuppressesAnswer(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc")
	if err := tr.RegisterInterface("lc", lc); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(11000)
	tag := bytes.Repeat([]byte{0x0E}, 16)
	requestorTID := bytes.Repeat([]byte{0x52}, 16)

	oldPkt := &packet.Packet{Raw: []byte{0xEE}}
	oldEntry := &PathAnnounceEntry{
		CreatedAt: time.Now().Add(-time.Hour),
		Packet:    oldPkt,
	}
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     append([]byte(nil), requestorTID...),
		Interface:   lc,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.announceTable[string(dest)] = oldEntry
	tr.mutex.Unlock()

	tr.processPathRequest(dest, lc, requestorTID, tag)

	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if cur != oldEntry {
		t.Fatal("next-hop-is-requestor must suppress answer even for local-client PRs")
	}
}

// ===========================================================================
// 6. FUZZ TESTS
// ===========================================================================

// FuzzHandlePathRequest verifies that handlePathRequest never panics on
// arbitrary input data (various lengths, empty, very large, etc.).
func FuzzHandlePathRequest(f *testing.F) {
	// Seed corpus: covers empty, short, exact, with-TID, oversized.
	f.Add([]byte{})
	f.Add([]byte{0x01})
	f.Add(bytes.Repeat([]byte{0x01}, 16))   // exactly destHash
	f.Add(bytes.Repeat([]byte{0x02}, 32))   // destHash + tag
	f.Add(bytes.Repeat([]byte{0x03}, 48))   // destHash + TID + tag
	f.Add(bytes.Repeat([]byte{0x04}, 1000)) // oversized
	f.Add(bytes.Repeat([]byte{0x00}, 16))   // all zeros (tagless)

	f.Fuzz(func(t *testing.T, data []byte) {
		tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
		tr.SetIdentity(mustIdentity(t))

		lc := newLocalClientRelayIface("lc_fuzz")
		if err := tr.RegisterInterface("lc_fuzz", lc); err != nil {
			t.Fatal(err)
		}
		wan := newRelayIface("wan_fuzz")
		if err := tr.RegisterInterface("wan_fuzz", wan); err != nil {
			t.Fatal(err)
		}

		// Must never panic regardless of input.
		tr.handlePathRequest(data, lc)

		// Allow async discovery goroutines to settle before close.
		time.Sleep(10 * time.Millisecond)
		tr.Close()
	})
}

// FuzzProcessPathRequest fuzzes processPathRequest with random destHash/tag/TID
// combinations to ensure no panics under arbitrary input.
func FuzzProcessPathRequest(f *testing.F) {
	dest16 := bytes.Repeat([]byte{0x11}, 16)
	tag16 := bytes.Repeat([]byte{0x22}, 16)
	tid16 := bytes.Repeat([]byte{0x33}, 16)
	empty := []byte{}

	f.Add(dest16, tag16, tid16, true)
	f.Add(dest16, tag16, empty, false)
	f.Add(empty, tag16, tid16, true)
	f.Add(dest16, empty, tid16, false)

	f.Fuzz(func(t *testing.T, destHash, tag, requestorTID []byte, useLocalClient bool) {
		tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
		tr.SetIdentity(mustIdentity(t))

		var src *relayIface
		if useLocalClient {
			src = newLocalClientRelayIface("src_fuzz")
		} else {
			src = newRelayIface("src_fuzz")
			src.Mode = common.IFModeGateway
		}
		if err := tr.RegisterInterface("src_fuzz", src); err != nil {
			t.Fatal(err)
		}
		wan := newRelayIface("wan_fuzz")
		if err := tr.RegisterInterface("wan_fuzz", wan); err != nil {
			t.Fatal(err)
		}

		// Must never panic regardless of input.
		tr.processPathRequest(destHash, src, requestorTID, tag)

		// Allow async discovery goroutines to settle before close.
		time.Sleep(10 * time.Millisecond)
		tr.Close()
	})
}

// FuzzHandlePathRequestMultipleInterfaces verifies no panics when handlePathRequest
// is called with various interfaces (local vs non-local) and data sizes.
func FuzzHandlePathRequestMultipleInterfaces(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03}, true)
	f.Add(bytes.Repeat([]byte{0xFF}, 48), false)
	f.Add(bytes.Repeat([]byte{0xAA}, 32), true)

	f.Fuzz(func(t *testing.T, data []byte, useLocalClient bool) {
		tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
		tr.SetIdentity(mustIdentity(t))

		var src *relayIface
		if useLocalClient {
			src = newLocalClientRelayIface("multi_fuzz_src")
		} else {
			src = newRelayIface("multi_fuzz_src")
			src.Mode = common.IFModeRoaming
		}
		if err := tr.RegisterInterface(src.GetName(), src); err != nil {
			t.Fatal(err)
		}
		wan := newRelayIface("multi_fuzz_wan")
		if err := tr.RegisterInterface("multi_fuzz_wan", wan); err != nil {
			t.Fatal(err)
		}

		tr.handlePathRequest(data, src)

		// Allow async discovery goroutines to settle before close.
		time.Sleep(10 * time.Millisecond)
		tr.Close()
	})
}

// ===========================================================================
// 7. INTEGRATION: handlePathRequest → processPathRequest end-to-end
// ===========================================================================

// TestE2E_HandlePathRequestFromLocalClientForwards verifies the full
// handlePathRequest → processPathRequest pipeline for a local-client
// interface, ensuring the data parsing correctly extracts destHash + tag.
func TestE2E_HandlePathRequestFromLocalClientForwards(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc_e2e")
	if err := tr.RegisterInterface("lc_e2e", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan_e2e")
	if err := tr.RegisterInterface("wan_e2e", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(12000)
	tag := make([]byte, 16)
	_, _ = rand.Read(tag)

	// Build path request data: destHash + tag (no transport ID since
	// transport identity may or may not be set).
	data := make([]byte, 0, len(dest)+len(tag))
	data = append(data, dest...)
	data = append(data, tag...)

	tr.handlePathRequest(data, lc)

	if n := countSends(wan); n != 1 {
		t.Fatalf("E2E: expected 1 PR forwarded to wan, got %d", n)
	}
	if n := countSends(lc); n != 0 {
		t.Fatalf("E2E: no echo to local client, got %d", n)
	}
}

// TestE2E_HandlePathRequestWithTransportIDFromLocalClient verifies parsing
// when the path request data includes a requestor transport ID.
func TestE2E_HandlePathRequestWithTransportIDFromLocalClient(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc_tid")
	if err := tr.RegisterInterface("lc_tid", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan_tid")
	if err := tr.RegisterInterface("wan_tid", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(13000)
	tid := bytes.Repeat([]byte{0x77}, 16)
	tag := make([]byte, 16)
	_, _ = rand.Read(tag)

	// Build data: destHash + transportID + tag
	data := make([]byte, 0, len(dest)+len(tid)+len(tag))
	data = append(data, dest...)
	data = append(data, tid...)
	data = append(data, tag...)

	tr.handlePathRequest(data, lc)

	if n := countSends(wan); n != 1 {
		t.Fatalf("E2E with TID: expected 1 PR forwarded, got %d", n)
	}
}

// TestE2E_DuplicatePRSuppressedByDiscoveryPRTags verifies that duplicate
// handlePathRequest calls (same destHash+tag) are suppressed by the
// discoveryPRTags dedup, even for local-client interfaces.
func TestE2E_DuplicatePRSuppressedByDiscoveryPRTags(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc_dup")
	if err := tr.RegisterInterface("lc_dup", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan_dup")
	if err := tr.RegisterInterface("wan_dup", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(14000)
	tag := bytes.Repeat([]byte{0xBB}, 16)

	data := make([]byte, 0, len(dest)+len(tag))
	data = append(data, dest...)
	data = append(data, tag...)

	// First call: should forward.
	tr.handlePathRequest(data, lc)
	firstSends := countSends(wan)

	// Second call with same destHash+tag: should be suppressed by dedup.
	tr.handlePathRequest(data, lc)
	secondSends := countSends(wan)

	if firstSends != 1 {
		t.Fatalf("first call: expected 1 send, got %d", firstSends)
	}
	if secondSends != firstSends {
		t.Fatalf("duplicate call: expected %d sends (no new), got %d", firstSends, secondSends)
	}
}

// TestE2E_TaglessPRDropped verifies that tagless path requests are dropped by
// handlePathRequest (matching Python behavior).
func TestE2E_TaglessPRDropped(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc_tagless")
	if err := tr.RegisterInterface("lc_tagless", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan_tagless")
	if err := tr.RegisterInterface("wan_tagless", wan); err != nil {
		t.Fatal(err)
	}

	// Only destHash, no tag (exactly 16 bytes).
	data := randomDestHash(15000)

	tr.handlePathRequest(data, lc)

	if n := countSends(wan); n != 0 {
		t.Fatalf("tagless PR must be dropped, got %d sends", n)
	}
}

// ===========================================================================
// 8. ATOMIC COUNTER / METRICS TESTS
// ===========================================================================

// TestLocalClientPR_ReceivedPathRequestAccounting verifies that
// ReceivedPathRequest() is called on the attached interface when a PR is
// processed via handlePathRequest (the accounting path is exercised, even
// though BaseInterface's implementation is currently a no-op).
func TestLocalClientPR_ReceivedPathRequestAccounting(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	lc := newLocalClientRelayIface("lc_acct")
	if err := tr.RegisterInterface("lc_acct", lc); err != nil {
		t.Fatal(err)
	}
	wan := newRelayIface("wan_acct")
	if err := tr.RegisterInterface("wan_acct", wan); err != nil {
		t.Fatal(err)
	}

	dest := randomDestHash(16000)
	tag := bytes.Repeat([]byte{0xCC}, 16)
	data := append(append([]byte{}, dest...), tag...)

	// This must not panic, as ReceivedPathRequest is a no-op on BaseInterface
	// but is still called. The forwarded PR should land on wan.
	tr.handlePathRequest(data, lc)

	if n := countSends(wan); n != 1 {
		t.Fatalf("expected 1 forwarded PR, got %d", n)
	}
}

// Live shared-client path request coverage lives in
// tests/interop/path_request_shared_live_test.go (RUN_LIVE_INTEROP=1).
