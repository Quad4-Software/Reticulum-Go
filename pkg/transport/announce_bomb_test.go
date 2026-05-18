// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
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

	pkt, err := CreateAnnouncePacket(destHash, id, maxApp, maxName, 0, nil)
	if err != nil {
		t.Fatalf("at-limit announce rejected: %v", err)
	}
	if len(pkt) == 0 {
		t.Fatal("empty announce packet")
	}
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
