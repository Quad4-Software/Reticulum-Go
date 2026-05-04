// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
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
	tr.paths[string(dest)] = &common.Path{
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

	oldPkt := &packet.Packet{Raw: []byte{0xCC, 0xDD}}
	oldEntry := &PathAnnounceEntry{
		CreatedAt: time.Now().Add(-time.Hour),
		Packet:    oldPkt,
	}

	tr.mutex.Lock()
	tr.paths[string(dest)] = &common.Path{
		NextHop:     append([]byte(nil), nextHop...),
		Interface:   wan,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.announceTable[string(dest)] = oldEntry
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, append([]byte(nil), requestorTID...), tag)

	tr.mutex.RLock()
	cur := tr.announceTable[string(dest)]
	_, held := tr.heldAnnounces[string(dest)]
	tr.mutex.RUnlock()
	if cur == nil {
		t.Fatal("expected new announce table entry")
	}
	if cur == oldEntry {
		t.Fatal("expected announce entry to be replaced for valid next hop")
	}
	if !held {
		t.Fatal("previous announce should be moved to heldAnnounces before replace")
	}
	if cur.Packet != oldPkt {
		t.Fatalf("replacement entry should carry same cached packet payload pointer: %p vs %p", cur.Packet, oldPkt)
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
	tr.paths[string(dest)] = &common.Path{
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

// TestProcessPathRequest_stalePathByTTLStartsDiscovery ensures path replies use the
// same freshness window as HasPath: an over-TTL path row is dropped and the
// request is treated as unknown (discovery state), not a silent no-op answer.
func TestProcessPathRequest_stalePathByTTLStartsDiscovery(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	wan := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", wan); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x45}, 16)
	tag := bytes.Repeat([]byte{0x91}, 16)
	nh := bytes.Repeat([]byte{0x62}, 16)

	tr.mutex.Lock()
	tr.paths[string(dest)] = &common.Path{
		NextHop:     nh,
		Interface:   wan,
		HopCount:    1,
		LastUpdated: time.Now().Add(-time.Duration(PathRequestTTL+10) * time.Second),
	}
	tr.mutex.Unlock()

	tr.processPathRequest(dest, wan, nil, tag)

	tr.mutex.RLock()
	_, ok := tr.discoveryPathRequests[string(dest)]
	tr.mutex.RUnlock()
	if !ok {
		t.Fatal("stale path by PathRequestTTL should fall through to discovery")
	}
}
