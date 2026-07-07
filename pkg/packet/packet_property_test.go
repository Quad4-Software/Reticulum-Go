// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package packet

import (
	"bytes"
	"testing"
)

func FuzzPacketRoundTrip(f *testing.F) {
	// Add some seed corpora
	f.Add(byte(HeaderType1), byte(PacketTypeData), byte(0), byte(ContextNone), byte(0), byte(0), []byte("hello"), []byte("0123456789abcdef"), []byte{})
	f.Add(byte(HeaderType2), byte(PacketTypeAnnounce), byte(1), byte(ContextResourceAdv), byte(1), byte(5), []byte("world"), []byte("0123456789abcdef"), []byte("1234567890abcdef"))

	f.Fuzz(func(t *testing.T, headerType, packetType, transportType, context, contextFlag, hops byte, data, destHash, transportID []byte) {
		// Sanitize inputs to valid ranges
		headerType %= 2
		packetType %= 4
		transportType %= 2
		contextFlag %= 2
		if len(destHash) != 16 {
			return
		}
		if headerType == HeaderType2 && len(transportID) != 16 {
			return
		}
		if headerType == HeaderType1 {
			transportID = nil
		}

		need := 2 + len(destHash) + 1 + len(data)
		if headerType == HeaderType2 {
			need += len(transportID)
		}
		if need > MTU {
			return
		}

		p := &Packet{
			HeaderType:      headerType,
			PacketType:      packetType,
			TransportType:   transportType,
			Context:         context,
			ContextFlag:     contextFlag,
			Hops:            hops,
			DestinationHash: destHash,
			TransportID:     transportID,
			Data:            data,
		}

		// Pack
		err := p.Pack()
		if err != nil {
			return
		}

		// Unpack
		p2 := &Packet{Raw: p.Raw}
		err = p2.Unpack()
		if err != nil {
			t.Fatalf("Unpack failed: %v", err)
		}

		// Property: Round-trip should preserve fields
		if p2.HeaderType != p.HeaderType {
			t.Errorf("HeaderType mismatch: %v != %v", p2.HeaderType, p.HeaderType)
		}
		if p2.PacketType != p.PacketType {
			t.Errorf("PacketType mismatch: %v != %v", p2.PacketType, p.PacketType)
		}
		if p2.TransportType != p.TransportType {
			t.Errorf("TransportType mismatch: %v != %v", p2.TransportType, p.TransportType)
		}
		if p2.Context != p.Context {
			t.Errorf("Context mismatch: %v != %v", p2.Context, p.Context)
		}
		if p2.ContextFlag != p.ContextFlag {
			t.Errorf("ContextFlag mismatch: %v != %v", p2.ContextFlag, p.ContextFlag)
		}
		if p2.Hops != p.Hops {
			t.Errorf("Hops mismatch: %v != %v", p2.Hops, p.Hops)
		}
		if !bytes.Equal(p2.DestinationHash, p.DestinationHash) {
			t.Errorf("DestinationHash mismatch: %x != %x", p2.DestinationHash, p.DestinationHash)
		}
		if !bytes.Equal(p2.Data, p.Data) {
			t.Errorf("Data mismatch: %x != %x", p2.Data, p.Data)
		}
		if headerType == HeaderType2 && !bytes.Equal(p2.TransportID, p.TransportID) {
			t.Errorf("TransportID mismatch: %x != %x", p2.TransportID, p.TransportID)
		}

		// Property: Hashing consistency
		h1 := p.GetHash()
		h2 := p2.GetHash()
		if !bytes.Equal(h1, h2) {
			t.Errorf("Hash mismatch after roundtrip: %x != %x", h1, h2)
		}
	})
}
