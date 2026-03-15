// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package link

import (
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

func FuzzLinkHandleData(f *testing.F) {
	// Setup a mock link
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	id, _ := identity.New()
	dest, _ := destination.New(id, destination.IN, destination.SINGLE, "testapp", tr, "service")
	
	l := NewLink(dest, tr, nil, nil, nil)
	l.status.Store(int32(STATUS_ACTIVE))
	l.linkID = make([]byte, 16)
	l.sessionKey = make([]byte, 32) // Dummy session key to trigger decryption logic

	// Add seeds
	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Context:         packet.ContextNone,
		Data:            []byte("hello world"),
	}
	if err := p.Pack(); err == nil {
		f.Add(p.Raw)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		pkt := &packet.Packet{Raw: data}
		if err := pkt.Unpack(); err != nil {
			return
		}
		
		// Ensure it's a link-type packet for the mock link
		pkt.DestinationType = DEST_TYPE_LINK
		pkt.DestinationHash = l.linkID

		// We just want to make sure it doesn't panic
		_ = l.handleDataPacket(pkt)
	})
}
