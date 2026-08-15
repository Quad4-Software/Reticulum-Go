// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"testing"
)

func TestBughuntPackRejectsShortDestinationHash(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationType: DestinationSingle,
		DestinationHash: make([]byte, 8),
		Data:            []byte("x"),
	}
	if err := p.Pack(); err == nil {
		t.Fatal("Pack accepted 8-byte destination hash")
	}
}

func TestBughuntPackAllowsEmptyDestinationHashForLocalHash(t *testing.T) {
	p := &Packet{Data: []byte("hash-me")}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
}

func TestBughuntPackRejectsShortTransportID(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		DestinationType: DestinationSingle,
		DestinationHash: make([]byte, TruncatedHashLength),
		TransportID:     make([]byte, 8),
		Data:            []byte("x"),
	}
	if err := p.Pack(); err == nil {
		t.Fatal("Pack accepted 8-byte transport ID")
	}
}
