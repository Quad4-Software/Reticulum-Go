// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

func FuzzLinkHandleData(f *testing.F) {
	// Setup a mock link
	cfg := &common.ReticulumConfig{}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	id, _ := identity.New()
	dest, _ := destination.New(id, destination.In, destination.Single, "testapp", tr, "service")

	l := NewLink(dest, tr, nil, nil, nil)
	l.status.Store(int32(StatusActive))
	l.linkID = make([]byte, 16)
	_ = setSecBuf(&l.sessionKey, make([]byte, 32)) // Dummy session key to trigger decryption logic
	_ = setSecBuf(&l.hmacKey, make([]byte, 32))

	// Add seeds
	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		DestinationType: DestTypeLink,
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
		pkt.DestinationType = DestTypeLink
		pkt.DestinationHash = l.linkID

		// We just want to make sure it doesn't panic
		_ = l.handleDataPacket(pkt)
	})
}
