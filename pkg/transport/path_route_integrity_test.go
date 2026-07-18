// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

// TestProcessPathRequest_doesNotAnswerWhenNextHopEqualsRequestorTransportID
// guards against relaying a path answer that would send traffic back toward
// the requester (the failure mode described for stale path_table + path request).
func TestProcessPathRequest_doesNotAnswerWhenNextHopEqualsRequestorTransportID(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	wan := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x31}, 16)
	requestorTID := bytes.Repeat([]byte{0x52}, 16)
	tag := bytes.Repeat([]byte{0x71}, 16)

	oldPkt := &packet.Packet{Raw: []byte{0xAA, 0xBB}}
	oldEntry := &PathAnnounceEntry{
		CreatedAt: time.Now().Add(-time.Hour),
		Packet:    oldPkt,
	}

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     append([]byte(nil), requestorTID...),
		Interface:   wan,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.announceTable[string(dest)] = oldEntry
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, append([]byte(nil), requestorTID...), tag)

	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	tr.mutex.RUnlock()
	if cur != oldEntry {
		t.Fatal("announce table entry must not be replaced when next hop is the requestor")
	}
	if cur.Packet != oldPkt {
		t.Fatal("announce packet must be unchanged")
	}
}

// TestProcessPathRequest_rewritesAnnounceWhenNextHopIsNotRequestor is a control:
// when the cached next hop is not the requestor, a path request may refresh the
// announce queue entry from the cached announce payload.
func TestProcessPathRequest_rewritesAnnounceWhenNextHopIsNotRequestor(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	wan := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x31}, 16)
	requestorTID := bytes.Repeat([]byte{0x52}, 16)
	nextHop := bytes.Repeat([]byte{0x61}, 16)
	tag := bytes.Repeat([]byte{0x72}, 16)

	oldPkt := &packet.Packet{
		DestinationHash: append([]byte(nil), dest...),
		Data:            []byte{0xCC, 0xDD},
	}
	oldEntry := &PathAnnounceEntry{
		CreatedAt:         time.Now().Add(-time.Hour),
		Packet:            oldPkt,
		AttachedInterface: wan, // in-flight delivery, should be held across replace
	}

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     append([]byte(nil), nextHop...),
		Interface:   wan,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.announcePacketCache[string(dest)] = oldPkt
	tr.announceTable[string(dest)] = oldEntry
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, append([]byte(nil), requestorTID...), tag)

	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	heldEntry, held := tr.heldAnnounces[string(dest)]
	tr.mutex.RUnlock()
	if cur == nil {
		t.Fatal("expected new announce table entry")
	}
	if cur == oldEntry {
		t.Fatal("expected announce entry to be replaced for valid next hop")
	}
	if !held || heldEntry != oldEntry {
		t.Fatal("previous in-flight announce should be moved to heldAnnounces before replace")
	}
	if cur.ReceivedFrom != wan {
		t.Fatal("replacement entry should record path egress interface as ReceivedFrom")
	}
}

// TestProcessPathRequest_knownPathWithoutAnnounceDoesNotStartDiscovery verifies that
// a path cache hit without a cached announce does not fall through to discovery
// (no synthetic answer and no discoveryPathRequests row).
func TestProcessPathRequest_knownPathWithoutAnnounceDoesNotStartDiscovery(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()

	wan := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x41}, 16)
	tag := bytes.Repeat([]byte{0x81}, 16)

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x62}, 16),
		Interface:   wan,
		HopCount:    1,
		LastUpdated: time.Now(),
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, nil, tag)

	tr.mutex.RLock()
	_, pending := tr.discoveryPathRequests[string(dest)]
	tr.mutex.RUnlock()
	if pending {
		t.Fatal("known path without announce must not create discoveryPathRequests")
	}
}

// TestForwardTransportPacket_dropsWhenEgressInterfaceEqualsIngress exercises the
// relay rule that avoids forwarding HeaderType2 back out the same interface the
// packet arrived on (prevents trivial bounce / asymmetry at this hop).
func TestForwardTransportPacket_dropsWhenEgressInterfaceEqualsIngress(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id := mustIdentity(t)
	tr.SetIdentity(id)

	same := newRelayIface("same")
	if err := tr.RegisterInterface("same", same); err != nil {
		t.Fatalf("register: %v", err)
	}

	destHash := bytes.Repeat([]byte{0xAA}, 16)
	nextHop := bytes.Repeat([]byte{0xBB}, 16)
	tr.UpdatePath(destHash, nextHop, "same", 2)

	raw := buildHT2Packet(id.Hash(), destHash, 0, []byte{0x01, 0x02})
	tr.HandlePacket(raw, same)

	time.Sleep(80 * time.Millisecond)
	if n := len(same.snapshot()); n != 0 {
		t.Fatalf("expected no relay onto same iface as ingress, got %d sends", n)
	}
}

// TestProcessPathRequest_expiredPathStartsDiscovery ensures path replies use
// PATHFINDER_E / Expires like HasPath: an expired path row is dropped and the
// request falls through to discovery.
func TestProcessPathRequest_expiredPathStartsDiscovery(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := mockIface("wan", true)
	wan.Mode = common.IFModeGateway
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x45}, 16)
	tag := bytes.Repeat([]byte{0x91}, 16)
	nh := bytes.Repeat([]byte{0x62}, 16)

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     nh,
		Interface:   wan,
		HopCount:    1,
		LastUpdated: time.Now().Add(-time.Duration(PathfinderE+10) * time.Second),
		Expires:     time.Now().Add(-10 * time.Second),
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, nil, tag)

	tr.mutex.RLock()
	_, ok := tr.discoveryPathRequests[string(dest)]
	tr.mutex.RUnlock()
	if !ok {
		t.Fatal("PATHFINDER_E-expired path should fall through to discovery")
	}
}

// TestProcessPathRequest_pathRequestTTLStillAnswers ensures paths older than
// PathRequestTTL but within PATHFINDER_E remain answerable (Python has_path).
func TestProcessPathRequest_pathRequestTTLStillAnswers(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x46}, 16)
	tag := bytes.Repeat([]byte{0x92}, 16)
	nh := bytes.Repeat([]byte{0x63}, 16)
	now := time.Now()

	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     nh,
		Interface:   wan,
		HopCount:    1,
		LastUpdated: now.Add(-time.Duration(PathRequestTTL+30) * time.Second),
		Expires:     now.Add(time.Duration(PathfinderE) * time.Second),
	}
	tr.announcePacketCache[string(dest)] = &packet.Packet{
		DestinationHash: append([]byte(nil), dest...),
		Data:            []byte{0x01},
		Raw:             []byte{0x01, 0x00},
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, nil, tag)

	tr.mutex.RLock()
	_, discovering := tr.discoveryPathRequests[string(dest)]
	_, hasPath := tr.paths[pathMapKey(dest)]
	tr.mutex.RUnlock()
	if discovering {
		t.Fatal("non-expired path must not be forced into discovery by PathRequestTTL")
	}
	if !hasPath {
		t.Fatal("path within PATHFINDER_E must remain")
	}
}

// TestProcessPathRequest_fromLocalClientForwardsOnAllInterfaces verifies that a
// path request arriving from a local (shared-instance) client interface is
// forwarded on all other registered interfaces, even when the local client
// interface has IFModeFull (which is NOT in DiscoverPathsFor). This matches
// Python RNS Transport.path_request "elif is_from_local_client" branch.
func TestProcessPathRequest_fromLocalClientForwardsOnAllInterfaces(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	// localClient simulates a spawned LocalClientInterface on the shared-instance
	// server side: IFTypeUnix + IFModeFull (default). IFModeFull is NOT in
	// DiscoverPathsFor, so without the is_from_local_client branch the PR would
	// be dropped.
	localClient := newRelayIface("local-client")
	localClient.Type = common.IFTypeUnix
	localClient.Mode = common.IFModeFull
	if err := tr.RegisterInterface("local-client", localClient); err != nil {
		t.Fatal(err)
	}

	wan := newRelayIface("wan")
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x99}, 16)
	tag := bytes.Repeat([]byte{0x77}, 16)

	tr.processPathRequest(dest, localClient, nil, tag)

	if n := len(wan.snapshot()); n != 1 {
		t.Fatalf("expected 1 PR forwarded to wan, got %d", n)
	}
	if n := len(localClient.snapshot()); n != 0 {
		t.Fatalf("PR must not be echoed back to the local client, got %d sends", n)
	}
}

// TestProcessPathRequest_fromLocalClientKnownPathBypassesTransportDisabled
// verifies that a path request from a local client for a known path is answered
// even when transport is disabled (matching Python's
// "(transport_enabled() or is_from_local_client) and has_path" condition).
func TestProcessPathRequest_fromLocalClientKnownPathBypassesTransportDisabled(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: false})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	localClient := newRelayIface("local-client")
	localClient.Type = common.IFTypeUnix
	localClient.Mode = common.IFModeFull
	if err := tr.RegisterInterface("local-client", localClient); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x44}, 16)
	tag := bytes.Repeat([]byte{0x88}, 16)

	oldPkt := &packet.Packet{
		DestinationHash: append([]byte(nil), dest...),
		Data:            []byte{0xAA, 0xBB},
	}
	tr.mutex.Lock()
	tr.paths[pathMapKey(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x62}, 16),
		Interface:   localClient,
		HopCount:    1,
		LastUpdated: time.Now(),
	}
	tr.announcePacketCache[string(dest)] = oldPkt
	tr.mutex.Unlock()

	// With transport disabled, a normal (non-local-client) PR would be
	// rejected. A local-client PR must still be answered.
	tr.processPathRequest(dest, localClient, nil, tag)

	if n := countSends(localClient); n < 1 {
		t.Fatalf("local-client PR with known path must emit path response, got %d sends", n)
	}
}

// TestProcessPathRequest_nonLocalClientDropsUnknownPathWithFullMode is a control
// test verifying that a PR from a non-local-client interface with IFModeFull for
// an unknown destination is still dropped (the old behavior is preserved for
// regular relayed PRs).
func TestProcessPathRequest_nonLocalClientDropsUnknownPathWithFullMode(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	fullIface := newRelayIface("full")
	fullIface.Mode = common.IFModeFull
	if err := tr.RegisterInterface("full", fullIface); err != nil {
		t.Fatal(err)
	}

	wan := newRelayIface("wan2")
	if err := tr.RegisterInterface("wan2", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x33}, 16)
	tag := bytes.Repeat([]byte{0x55}, 16)

	tr.processPathRequest(dest, fullIface, nil, tag)

	if n := len(wan.snapshot()); n != 0 {
		t.Fatalf("non-local-client PR with Full mode must NOT be forwarded, got %d sends", n)
	}
}
