// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"
)

// Guarantee: value-copying a hashed Packet then rehashing the copy must not
// overwrite the original packet's PacketHash buffer.
func TestOraclePacketHashBufNoAliasAfterValueCopy(t *testing.T) {
	dest := bytes.Repeat([]byte{0x11}, TruncatedHashLength)
	orig := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationType: DestinationSingle,
		DestinationHash: append([]byte(nil), dest...),
		Data:            []byte("alias-a"),
	}
	if err := orig.Pack(); err != nil {
		t.Fatalf("pack orig: %v", err)
	}
	h1 := append([]byte(nil), orig.GetHash()...)

	copied := *orig
	copied.Data = []byte("alias-b")
	copied.Packed = false
	copied.hashValid = false
	if err := copied.Pack(); err != nil {
		t.Fatalf("pack copy: %v", err)
	}
	h2 := copied.GetHash()

	if bytes.Equal(h1, h2) {
		t.Fatal("expected different hashes for different payloads")
	}
	if !bytes.Equal(orig.GetHash(), h1) {
		t.Fatalf("orig hash mutated after copy rehash: got %x want %x", orig.GetHash(), h1)
	}
	if len(orig.PacketHash) == 0 || len(copied.PacketHash) == 0 {
		t.Fatal("expected PacketHash to be set")
	}
	if &orig.PacketHash[0] == &copied.PacketHash[0] {
		t.Fatal("PacketHash slices alias across value-copied packets")
	}
}
