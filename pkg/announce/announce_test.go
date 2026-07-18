// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package announce

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
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

// TestHandleAnnouncePythonNoRatchet accepts RNS 1.3.8 announces that omit the
// ratchet field (context flag unset). Previously rejected as too short.
func TestHandleAnnouncePythonNoRatchet(t *testing.T) {
	cases := []struct {
		name string
		hex  string
	}{
		{
			name: "app_abc",
			hex:  "0100ed2ade34633329fc52260758a003b2d700742cb85deb480a60732e25b9d7620b1e3f6311c19da917567da17f06b847d025238517a60f02ec39cf523086b84a2b64eb25dc0d5d612189631fdaeec356e4c9512ef1d6ad58488aa5977e8359f9b8006a5aca99384d928a771e4350c7a7c119a289cabd93f457e69e6ac7e8a6f0b3d4979e3346b5218648bff82c75ac86a1de4dd6aaad41041b96ccb1714f6fcc96e3b0917700616263",
		},
		{
			name: "empty_app",
			hex:  "01006f60d82aadb374feb4647c0d25d121b900221aeabc3a1f76304c6a1d19ce34e4ddf7b0240222c87d76aa310034756779648b4e9fab29a2accacda525e9aa5bf59c3007bf43fc6a00dc091775d0885551be1f1eb3b5c1190d17e3e8d638c6b151006a5aca9998c72203256ceb27bb2b9918b46f438f5ecf48efc345513a38fc675a982dd2a3e1369331efd0aac2859ef790c30434c4ba82e2c1e0ef49a3e539881083b33102",
		},
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	ann, err := New(id, make([]byte, 16), "pyann", nil, false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.hex)
			if err != nil {
				t.Fatal(err)
			}
			if err := ann.HandleAnnounce(raw); err != nil {
				t.Fatalf("HandleAnnounce: %v", err)
			}
		})
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
