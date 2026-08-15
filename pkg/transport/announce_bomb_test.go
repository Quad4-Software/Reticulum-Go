// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

func TestCreateAnnouncePacket_RejectsOversizeName(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xAA}, 16)
	hugeName := strings.Repeat("a", MsgpackBin8MaxLen+1)

	_, err = CreateAnnouncePacket(destHash, id, []byte("ok"), hugeName, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "msgpack bin8 limit") {
		t.Fatalf("expected oversize-name rejection, got: %v", err)
	}
}

func TestCreateAnnouncePacket_RejectsOversizeAppData(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xBB}, 16)
	hugeApp := bytes.Repeat([]byte{0x01}, MsgpackBin8MaxLen+1)

	_, err = CreateAnnouncePacket(destHash, id, hugeApp, "node", 0, nil)
	if err == nil || !strings.Contains(err.Error(), "msgpack bin8 limit") {
		t.Fatalf("expected oversize-appData rejection, got: %v", err)
	}
}

func TestCreateAnnouncePacket_AcceptsAtBin8Limit(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xCC}, 16)
	maxName := strings.Repeat("n", MsgpackBin8MaxLen)
	maxApp := bytes.Repeat([]byte{0xDE}, MsgpackBin8MaxLen)

	_, err = CreateAnnouncePacket(destHash, id, maxApp, maxName, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds MTU") {
		t.Fatalf("expected MTU rejection for max bin8 name+appData, got: %v", err)
	}
}

func TestCreateAnnouncePacket_AcceptsLargestFitWithinMTU(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xCD}, 16)
	name := "node"
	// Probe down from bin8 max until the wire form fits packet.MTU.
	appLen := MsgpackBin8MaxLen
	var pkt []byte
	for appLen > 0 {
		app := bytes.Repeat([]byte{0xAB}, appLen)
		pkt, err = CreateAnnouncePacket(destHash, id, app, name, 0, nil)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "exceeds MTU") {
			t.Fatalf("unexpected error at appLen=%d: %v", appLen, err)
		}
		appLen--
	}
	if err != nil {
		t.Fatalf("no within-MTU size found: %v", err)
	}
	if len(pkt) > packet.MTU {
		t.Fatalf("packet %d exceeds MTU %d", len(pkt), packet.MTU)
	}
	t.Logf("largest fitting app_data=%d packet=%d", appLen, len(pkt))
}

func TestCreateAnnouncePacket_RejectsAlmostBin16Size(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xDD}, 16)
	bin16ish := bytes.Repeat([]byte{0xEE}, 65535)

	_, err = CreateAnnouncePacket(destHash, id, bin16ish, "node", 0, nil)
	if err == nil || !strings.Contains(err.Error(), "msgpack bin8 limit") {
		t.Fatalf("expected bin16-sized appData rejection, got: %v", err)
	}
}

func TestCreateAnnouncePacket_FitsWithinPacketMTU(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xEF}, 16)

	pkt, err := CreateAnnouncePacket(destHash, id, []byte("typical-app-data"), "typical.node", 0, nil)
	if err != nil {
		t.Fatalf("CreateAnnouncePacket: %v", err)
	}
	if len(pkt) > packet.MTU {
		t.Fatalf("typical announce %d bytes exceeds packet.MTU=%d", len(pkt), packet.MTU)
	}
}
