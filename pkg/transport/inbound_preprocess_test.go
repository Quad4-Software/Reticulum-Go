// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/packet"
)

func TestPreprocessMinimalDataPacket(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	raw := minimalDataPacket()
	pkt := &packet.Packet{Raw: raw}
	if err := pkt.Unpack(); err != nil {
		t.Fatalf("unpack: %v len=%d", err, len(raw))
	}
	iface := newOnlineTestIface("x")
	_, _, ok := tr.preprocessInboundPacket(raw, iface)
	if !ok {
		t.Fatal("preprocess failed")
	}
}
