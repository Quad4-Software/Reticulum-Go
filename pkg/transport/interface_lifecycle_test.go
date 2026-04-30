// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

func mockIface(name string, enabled bool) *mockInterface {
	m := &mockInterface{}
	m.Name = name
	m.Enabled = enabled
	return m
}

func TestUnregisterInterfaceScrubsPaths(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	a := mockIface("a", true)
	b := mockIface("b", true)
	if err := tr.RegisterInterface("a", a); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("b", b); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0xAB}, 16)
	tr.mutex.Lock()
	tr.paths[string(dest)] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x01}, 16),
		Interface:   b,
		HopCount:    1,
		LastUpdated: time.Now(),
	}
	tr.mutex.Unlock()

	tr.UnregisterInterface("b")

	if tr.HasPath(dest) {
		t.Fatal("path should be removed when egress interface unregisters")
	}
	if tr.NextHopInterface(dest) != "" {
		t.Fatalf("NextHopInterface want empty, got %q", tr.NextHopInterface(dest))
	}
}

func TestUnregisterInterfaceScrubsLinkRelay(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	a := mockIface("a", true)
	b := mockIface("b", true)
	if err := tr.RegisterInterface("a", a); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("b", b); err != nil {
		t.Fatal(err)
	}

	linkID := bytes.Repeat([]byte{0xCD}, identity.TruncatedHashLength/8)
	tr.mutex.Lock()
	tr.paths[string(bytes.Repeat([]byte{0x11}, 16))] = &common.Path{
		NextHop:     bytes.Repeat([]byte{0x22}, 16),
		Interface:   b,
		HopCount:    2,
		LastUpdated: time.Now(),
	}
	tr.mutex.Unlock()

	tr.linkTable.put(linkID, &LinkRelayEntry{
		NextHop:         bytes.Repeat([]byte{0x22}, 16),
		NextHopIface:    b,
		ReceivedIface:   a,
		RemainingHops:   2,
		TakenHops:       0,
		DestinationHash: bytes.Repeat([]byte{0x33}, 16),
		Validated:       false,
		ProofTimeout:    time.Now().Add(time.Hour),
		Timestamp:       time.Now(),
		OriginalLinkID:  append([]byte(nil), linkID...),
	})

	tr.UnregisterInterface("b")
	if _, ok := tr.linkTable.get(linkID); ok {
		t.Fatal("link relay entry referencing removed interface should be dropped")
	}
}

func TestUnregisterInterfaceScrubsDiscoveryAndAnnounceTables(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	a := mockIface("wan", true)
	if err := tr.RegisterInterface("wan", a); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x55}, 16)
	tr.mutex.Lock()
	tr.discoveryPathRequests[string(dest)] = &DiscoveryPathRequest{
		DestinationHash: dest,
		Timeout:         time.Now().Add(time.Hour),
		RequestingIface: a,
	}
	tr.announceTable["x"] = &PathAnnounceEntry{
		CreatedAt:    time.Now(),
		ReceivedFrom: a,
	}
	tr.heldAnnounces["y"] = &PathAnnounceEntry{
		CreatedAt:         time.Now(),
		AttachedInterface: a,
	}
	tr.mutex.Unlock()

	tr.UnregisterInterface("wan")

	tr.mutex.RLock()
	_, dOK := tr.discoveryPathRequests[string(dest)]
	_, aOK := tr.announceTable["x"]
	_, hOK := tr.heldAnnounces["y"]
	tr.mutex.RUnlock()

	if dOK || aOK || hOK {
		t.Fatalf("expected tables cleared: discovery=%v announce=%v held=%v", dOK, aOK, hOK)
	}
}

func TestReplaceInterfaceSwapsRegistration(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	first := mockIface("u1", true)
	second := mockIface("u1", true)
	if err := tr.RegisterInterface("u1", first); err != nil {
		t.Fatal(err)
	}
	dest := bytes.Repeat([]byte{0x77}, 16)
	tr.UpdatePath(dest, bytes.Repeat([]byte{0x88}, 16), "u1", 1)

	if err := tr.ReplaceInterface("u1", second); err != nil {
		t.Fatal(err)
	}
	if tr.HasPath(dest) {
		t.Fatal("path tied to old iface pointer should be cleared on replace")
	}
	got, err := tr.GetInterface("u1")
	if err != nil || got != second {
		t.Fatalf("GetInterface: err=%v got=%p want second %p", err, got, second)
	}
}

func TestRequestPathSkipsDisabledInterface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	up := mockIface("up", true)
	down := mockIface("down", false)
	if err := tr.RegisterInterface("up", up); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("down", down); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x99}, 16)
	if err := tr.RequestPath(dest, "", nil, false); err != nil {
		t.Fatal(err)
	}
	if len(up.sent) == 0 {
		t.Fatal("expected path request on enabled interface")
	}
	if len(down.sent) != 0 {
		t.Fatal("disabled interface should not send path request")
	}
}

func TestUnregisterInterfaceIdempotent(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	tr.UnregisterInterface("nonexistent")
}

func TestSendAnnounceSkipsDisabledInterface(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	SetTransportInstance(tr)
	defer SetTransportInstance(nil)

	up := mockIface("up", true)
	down := mockIface("down", false)
	if err := tr.RegisterInterface("up", up); err != nil {
		t.Fatal(err)
	}
	if err := tr.RegisterInterface("down", down); err != nil {
		t.Fatal(err)
	}

	pkt := []byte{0x01, 0x02, 0x03}
	if err := SendAnnounce(pkt); err != nil {
		t.Fatal(err)
	}
	if len(up.sent) == 0 {
		t.Fatal("enabled iface should receive announce send")
	}
	if len(down.sent) != 0 {
		t.Fatal("disabled iface should not send")
	}
}
