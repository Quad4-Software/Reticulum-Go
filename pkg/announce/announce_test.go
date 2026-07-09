// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

type mockAnnounceHandler struct {
	received bool
}

func (m *mockAnnounceHandler) AspectFilter() []string {
	return nil
}

func (m *mockAnnounceHandler) ReceivedAnnounce(destinationHash []byte, announcedIdentity any, appData []byte, hops uint8) error {
	m.received = true
	return nil
}

func (m *mockAnnounceHandler) ReceivePathResponses() bool {
	return true
}

type mockInterface struct {
	common.BaseInterface
	sent bool
}

func (m *mockInterface) Send(data []byte, address string) error {
	m.sent = true
	return nil
}

func (m *mockInterface) GetBandwidthAvailable() bool {
	return true
}

func (m *mockInterface) IsEnabled() bool {
	return true
}

func TestNewAnnounce(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, err := New(id, destHash, "testapp", []byte("appdata"), false, config)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if ann == nil {
		t.Fatal("New returned nil")
	}

	if !bytes.Equal(ann.destinationHash, destHash) {
		t.Error("Destination hash doesn't match")
	}
}

func TestCreateAndHandleAnnounce(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, _ := New(id, destHash, "testapp", []byte("appdata"), false, config)
	packet, err := ann.CreatePacket()
	if err != nil {
		t.Fatalf("CreatePacket: %v", err)
	}

	handler := &mockAnnounceHandler{}
	ann.RegisterHandler(handler)

	if err = ann.HandleAnnounce(packet); err != nil {
		t.Fatalf("HandleAnnounce failed: %v", err)
	}

	if !handler.received {
		t.Error("Handler did not receive announce")
	}
}

func TestPropagate(t *testing.T) {
	id, _ := identity.New()
	destHash := make([]byte, 16)
	config := &common.ReticulumConfig{}

	ann, _ := New(id, destHash, "testapp", []byte("appdata"), false, config)

	iface := &mockInterface{}
	iface.Name = "testiface"
	iface.Online = true
	iface.Enabled = true

	err := ann.Propagate([]common.NetworkInterface{iface})
	if err != nil {
		t.Fatalf("Propagate failed: %v", err)
	}

	if !iface.sent {
		t.Error("Packet was not sent on interface")
	}
}

func TestRandomHashTimestampEncoding(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := make([]byte, 16)
	cfg := &common.ReticulumConfig{}

	ann, err := New(id, destHash, "testapp", []byte("appdata"), false, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	before := time.Now().Unix()
	pkt, err := ann.CreatePacket()
	if err != nil {
		t.Fatalf("CreatePacket: %v", err)
	}
	after := time.Now().Unix()

	const randomHashStart = HeaderType1Offset + AnnounceRandomOffset
	if len(pkt) < randomHashStart+RandomHashSize {
		t.Fatalf("packet too short for random hash: len=%d", len(pkt))
	}
	tsBytes := pkt[randomHashStart+5 : randomHashStart+RandomHashSize]
	if len(tsBytes) != 5 {
		t.Fatalf("expected 5 timestamp bytes, got %d", len(tsBytes))
	}

	padded := make([]byte, 8)
	copy(padded[8-len(tsBytes):], tsBytes)
	got := int64(binary.BigEndian.Uint64(padded)) // #nosec G115

	if got < before || got > after {
		t.Fatalf("decoded announce timestamp %d outside expected window [%d,%d]; raw bytes=%x (encoding bug would yield %d)",
			got, before, after, tsBytes, int64(tsBytes[0]))
	}
}

// TestRandomHashChangesPerAnnounce ensures each announce packet
// produced by a destination carries a strictly different random
// hash, so a peer that deduplicates by packet hash cannot
// accidentally drop all follow-up announces from the same
// destination.
func TestRandomHashChangesPerAnnounce(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := make([]byte, 16)
	cfg := &common.ReticulumConfig{}

	const randomHashStart = HeaderType1Offset + AnnounceRandomOffset
	const iterations = 8

	prev := make(map[string]struct{}, iterations)
	for i := range iterations {
		ann, err := New(id, destHash, "testapp", []byte("appdata"), false, cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		pkt, err := ann.CreatePacket()
		if err != nil {
			t.Fatalf("CreatePacket: %v", err)
		}
		if len(pkt) < randomHashStart+RandomHashSize {
			t.Fatalf("packet too short")
		}
		rh := string(pkt[randomHashStart : randomHashStart+RandomHashSize])
		if _, dup := prev[rh]; dup {
			t.Fatalf("duplicate random hash in iteration %d: %x", i, []byte(rh))
		}
		prev[rh] = struct{}{}
	}
}

func TestHandlerRegistration(t *testing.T) {
	ann := &Announce{
		mutex: &sync.RWMutex{},
	}
	handler := &mockAnnounceHandler{}

	ann.RegisterHandler(handler)
	if len(ann.handlers) != 1 {
		t.Errorf("Expected 1 handler, got %d", len(ann.handlers))
	}

	ann.DeregisterHandler(handler)
	if len(ann.handlers) != 0 {
		t.Errorf("Expected 0 handlers, got %d", len(ann.handlers))
	}
}
