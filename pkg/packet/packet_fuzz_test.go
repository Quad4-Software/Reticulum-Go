// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package packet

import (
	"testing"
)

func FuzzPacketUnpack(f *testing.F) {
	// Add some valid packets as seeds
	p1 := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationType: 0x01,
		DestinationHash: make([]byte, 16),
		Context:         ContextNone,
		Data:            []byte("hello"),
	}
	if err := p1.Pack(); err == nil {
		f.Add(p1.Raw)
	}

	p2 := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeAnnounce,
		TransportID:     make([]byte, 16),
		DestinationHash: make([]byte, 16),
		Context:         ContextNone,
		Data:            []byte("announce"),
	}
	if err := p2.Pack(); err == nil {
		f.Add(p2.Raw)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		p := &Packet{Raw: data}
		// We don't care about the error, just that it doesn't panic
		_ = p.Unpack()
	})
}

