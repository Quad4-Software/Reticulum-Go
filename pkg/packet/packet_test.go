// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"crypto/rand"
	"fmt"
	mathrand "math/rand"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/identity"
)

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic("Failed to generate random bytes: " + err.Error())
	}
	return b
}

func TestPacketPackUnpack(t *testing.T) {
	testCases := []struct {
		name             string
		headerType       byte
		packetType       byte
		transportType    byte
		destType         byte
		context          byte
		contextFlag      byte
		dataSize         int
		needsTransportID bool
	}{
		{
			name:             "HeaderType1_Data_NoContextFlag",
			headerType:       HeaderType1,
			packetType:       PacketTypeData,
			transportType:    0x01, // Example
			destType:         0x02, // Example
			context:          ContextNone,
			contextFlag:      FlagUnset,
			dataSize:         100,
			needsTransportID: false,
		},
		{
			name:             "HeaderType2_Announce_ContextFlagSet",
			headerType:       HeaderType2,
			packetType:       PacketTypeAnnounce,
			transportType:    0x01, // Changed from 0x0F (15) to 1 (valid 1-bit value)
			destType:         0x01, // Example
			context:          ContextResourceAdv,
			contextFlag:      FlagSet,
			dataSize:         50,
			needsTransportID: true,
		},
		{
			name:             "HeaderType1_EmptyData",
			headerType:       HeaderType1,
			packetType:       PacketTypeProof,
			transportType:    0x00,
			destType:         0x00,
			context:          ContextLRProof,
			contextFlag:      FlagSet,
			dataSize:         0,
			needsTransportID: false,
		},
		{
			name:             "HeaderType2_MaxHops", // Hops are set manually before pack
			headerType:       HeaderType2,
			packetType:       PacketTypeLinkReq,
			transportType:    0x01, // Changed from 0x05 (5) to 1 (valid 1-bit value)
			destType:         0x03,
			context:          ContextLinkIdentify,
			contextFlag:      FlagUnset,
			dataSize:         200,
			needsTransportID: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalData := randomBytes(tc.dataSize)
			originalDestHash := randomBytes(16) // Truncated dest hash
			var originalTransportID []byte
			if tc.needsTransportID {
				originalTransportID = randomBytes(16)
			}

			p := &Packet{
				HeaderType:      tc.headerType,
				PacketType:      tc.packetType,
				TransportType:   tc.transportType,
				Context:         tc.context,
				ContextFlag:     tc.contextFlag,
				Hops:            5, // Example hops
				DestinationType: tc.destType,
				DestinationHash: originalDestHash,
				TransportID:     originalTransportID,
				Data:            originalData,
				Packed:          false,
			}

			err := p.Pack()
			if err != nil {
				t.Fatalf("Pack() failed: %v", err)
			}
			if !p.Packed {
				t.Error("Pack() did not set Packed flag to true")
			}
			if len(p.Raw) == 0 {
				t.Error("Pack() resulted in empty Raw data")
			}

			unpackTarget := &Packet{Raw: p.Raw}

			err = unpackTarget.Unpack()
			if err != nil {
				t.Fatalf("Unpack() failed: %v", err)
			}

			if unpackTarget.HeaderType != tc.headerType {
				t.Errorf("Unpacked HeaderType = %d; want %d", unpackTarget.HeaderType, tc.headerType)
			}
			if unpackTarget.PacketType != tc.packetType {
				t.Errorf("Unpacked PacketType = %d; want %d", unpackTarget.PacketType, tc.packetType)
			}
			if unpackTarget.TransportType != tc.transportType {
				t.Errorf("Unpacked TransportType = %d; want %d", unpackTarget.TransportType, tc.transportType)
			}
			if unpackTarget.Context != tc.context {
				t.Errorf("Unpacked Context = %d; want %d", unpackTarget.Context, tc.context)
			}
			if unpackTarget.ContextFlag != tc.contextFlag {
				t.Errorf("Unpacked ContextFlag = %d; want %d", unpackTarget.ContextFlag, tc.contextFlag)
			}
			if unpackTarget.Hops != 5 { // Should match the Hops set before packing
				t.Errorf("Unpacked Hops = %d; want %d", unpackTarget.Hops, 5)
			}
			if unpackTarget.DestinationType != tc.destType {
				t.Errorf("Unpacked DestinationType = %d; want %d", unpackTarget.DestinationType, tc.destType)
			}
			if !bytes.Equal(unpackTarget.DestinationHash, originalDestHash) {
				t.Errorf("Unpacked DestinationHash = %x; want %x", unpackTarget.DestinationHash, originalDestHash)
			}
			if !bytes.Equal(unpackTarget.Data, originalData) {
				t.Errorf("Unpacked Data = %x; want %x", unpackTarget.Data, originalData)
			}

			if tc.needsTransportID {
				if !bytes.Equal(unpackTarget.TransportID, originalTransportID) {
					t.Errorf("Unpacked TransportID = %x; want %x", unpackTarget.TransportID, originalTransportID)
				}
			} else {
				if unpackTarget.TransportID != nil {
					t.Errorf("Unpacked TransportID = %x; want nil", unpackTarget.TransportID)
				}
			}
		})
	}
}

func TestPackMTUExceeded(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            randomBytes(MTU + 10), // Exceed MTU
	}
	err := p.Pack()
	if err == nil {
		t.Errorf("Pack() should have failed due to exceeding MTU, but it didn't")
	}
}

func TestUnpackTooShort(t *testing.T) {
	testCases := []struct {
		name string
		raw  []byte
	}{
		{"VeryShort", []byte{0x01}},
		{"HeaderType1MinShort", []byte{0x00, 0x05, 0x01, 0x02}}, // Missing parts of dest hash
		{"HeaderType2MinShort", []byte{0x40, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}}, // Missing dest hash
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Packet{Raw: tc.raw}
			err := p.Unpack()
			if err == nil {
				t.Errorf("Unpack() should have failed for short packet, but it didn't")
			}
		})
	}
}

// TestUnpackRejectsPathfinderMHops mirrors RNS 1.3.8 Packet.unpack hop gate.
func TestUnpackRejectsPathfinderMHops(t *testing.T) {
	dest := randomBytes(16)
	for _, hops := range []byte{PathfinderM, 200, 255} {
		raw := make([]byte, 0, MinPacketSize+len(dest))
		raw = append(raw, 0x00, hops)
		raw = append(raw, dest...)
		raw = append(raw, ContextNone)
		p := &Packet{Raw: raw}
		if err := p.Unpack(); err == nil {
			t.Fatalf("Unpack hops=%d: want error, got nil", hops)
		}
	}
	raw := make([]byte, 0, MinPacketSize+len(dest))
	raw = append(raw, 0x00, PathfinderM-1)
	raw = append(raw, dest...)
	raw = append(raw, ContextNone)
	p := &Packet{Raw: raw}
	if err := p.Unpack(); err != nil {
		t.Fatalf("Unpack hops=%d: %v", PathfinderM-1, err)
	}
}

func TestPacketHashing(t *testing.T) {
	data := randomBytes(50)
	destHash := randomBytes(16)
	p1 := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0x01,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            2,
		DestinationType: 0x02,
		DestinationHash: destHash,
		Data:            data,
	}
	p2 := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		TransportType:   0x01,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            2,
		DestinationType: 0x02,
		DestinationHash: destHash,
		Data:            data,
	}

	if err := p1.Pack(); err != nil {
		t.Fatalf("p1.Pack() failed: %v", err)
	}
	if err := p2.Pack(); err != nil {
		t.Fatalf("p2.Pack() failed: %v", err)
	}

	hash1 := p1.GetHash()
	hash2 := p2.GetHash()
	if !bytes.Equal(hash1, hash2) {
		t.Errorf("Hashes of identical packets differ:\nHash1: %x\nHash2: %x", hash1, hash2)
	}
	if !bytes.Equal(p1.PacketHash, hash1) {
		t.Errorf("p1.PacketHash (%x) does not match GetHash() (%x)", p1.PacketHash, hash1)
	}

	p2.Hops = 3
	p2.Raw[1] = 3 // modify Raw directly as Pack is not re-called
	hash3 := p2.GetHash()
	if !bytes.Equal(hash1, hash3) {
		t.Errorf("Hash changed after modifying non-hashable Hops field:\nHash1: %x\nHash3: %x", hash1, hash3)
	}

	p2.Data = append(p2.Data, 0x99)
	p2.Raw = append(p2.Raw, 0x99)
	hash4 := p2.GetHash()
	if bytes.Equal(hash1, hash4) {
		t.Errorf("Hash did not change after modifying hashable Data field")
	}

	// Test HeaderType2 hashing difference
	p3 := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportType:   0x01,
		Context:         ContextNone,
		ContextFlag:     FlagUnset,
		Hops:            2,
		DestinationType: 0x02,
		DestinationHash: destHash,
		TransportID:     randomBytes(16),
		Data:            data,
	}
	if err := p3.Pack(); err != nil {
		t.Fatalf("p3.Pack() failed: %v", err)
	}
	hash5 := p3.GetHash()
	_ = hash5 // Use hash5 to avoid unused variable error
}

// BenchmarkPacketOperations benchmarks packet creation, packing, and hashing
func BenchmarkPacketOperations(b *testing.B) {
	// Prepare test data (keep under MTU limit)
	data := randomBytes(256)
	transportID := randomBytes(16)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create packet
		packet := NewPacket(0x00, data, PacketTypeData, ContextNone, 0x00, HeaderType1, transportID, false, 0x00)

		// Pack the packet
		if err := packet.Pack(); err != nil {
			b.Fatalf("Packet.Pack() failed: %v", err)
		}

		// Get hash (triggers crypto operations)
		_ = packet.GetHash()
	}
}

// BenchmarkPacketSerializeDeserialize benchmarks the full pack/unpack cycle
func BenchmarkPacketSerializeDeserialize(b *testing.B) {
	// Prepare test data (keep under MTU limit)
	data := randomBytes(256)
	transportID := randomBytes(16)

	// Create and pack original packet
	originalPacket := NewPacket(0x00, data, PacketTypeData, ContextNone, 0x00, HeaderType1, transportID, false, 0x00)
	if err := originalPacket.Pack(); err != nil {
		b.Fatalf("Original packet.Pack() failed: %v", err)
	}

	wireLen := len(originalPacket.Raw)
	buf := make([]byte, wireLen, nextRawWireCap(wireLen))
	copy(buf, originalPacket.Raw)
	packet := &Packet{Raw: buf}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copy(buf, originalPacket.Raw)
		packet.Raw = buf[:wireLen]

		if err := packet.Unpack(); err != nil {
			b.Fatalf("Packet.Unpack() failed: %v", err)
		}

		if err := packet.Pack(); err != nil {
			b.Fatalf("Packet.Pack() failed: %v", err)
		}
	}
}

func randomHash16(r *mathrand.Rand) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

func genValidPacket(r *mathrand.Rand, size int) *Packet {
	headerType := byte(r.Intn(2))
	packetType := byte(r.Intn(4))
	transportType := byte(r.Intn(2))
	context := byte(r.Intn(256))
	contextFlag := byte(r.Intn(2))
	hops := byte(r.Intn(PathfinderM))
	dest := randomHash16(r)
	var tid []byte
	if headerType == HeaderType2 {
		tid = randomHash16(r)
	}
	overhead := 19
	if headerType == HeaderType2 {
		overhead = 35
	}
	maxData := max(MTU-overhead, 0)
	if size > 0 && size < maxData {
		maxData = size
	}
	dataLen := r.Intn(maxData + 1)
	data := make([]byte, dataLen)
	for i := range data {
		data[i] = byte(r.Intn(256))
	}
	return &Packet{
		HeaderType:      headerType,
		PacketType:      packetType,
		TransportType:   transportType,
		Context:         context,
		ContextFlag:     contextFlag,
		Hops:            hops,
		DestinationHash: dest,
		TransportID:     tid,
		Data:            data,
	}
}

func TestPBTPacketPackUnpackRoundTrip(t *testing.T) {
	gen := pbt.NewGenerator("validPacket", genValidPacket)
	prop := pbt.ForAll(
		"pack unpack preserves fields and hash",
		gen,
		func(p *Packet) bool {
			if err := p.Pack(); err != nil {
				return false
			}
			p2 := &Packet{Raw: p.Raw}
			if err := p2.Unpack(); err != nil {
				return false
			}
			if p2.HeaderType != p.HeaderType ||
				p2.PacketType != p.PacketType ||
				p2.TransportType != p.TransportType ||
				p2.Context != p.Context ||
				p2.ContextFlag != p.ContextFlag ||
				p2.Hops != p.Hops {
				return false
			}
			if !bytes.Equal(p2.DestinationHash, p.DestinationHash) ||
				!bytes.Equal(p2.Data, p.Data) {
				return false
			}
			if p.HeaderType == HeaderType2 && !bytes.Equal(p2.TransportID, p.TransportID) {
				return false
			}
			h1 := p.GetHash()
			h2 := p2.GetHash()
			return bytes.Equal(h1, h2)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(42), pbt.WithMaxSize(450))
}

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
		if int(hops) >= PathfinderM {
			// Pack may still emit the byte, but unpack must reject it (RNS 1.3.8).
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
			if err := p.Pack(); err != nil {
				return
			}
			p2 := &Packet{Raw: p.Raw}
			if err := p2.Unpack(); err == nil {
				t.Fatalf("Unpack should reject hops=%d", hops)
			}
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

func BenchmarkPacketThroughput(b *testing.B) {
	payloadSizes := []int{16, 64, 256, 450} // 450 is near MTU

	for _, size := range payloadSizes {
		b.Run(fmt.Sprintf("Payload-%d", size), func(b *testing.B) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i)
			}

			p := &Packet{
				HeaderType:      HeaderType1,
				PacketType:      PacketTypeData,
				DestinationHash: make([]byte, 16),
				Data:            data,
			}

			b.ResetTimer()
			b.ReportAllocs()

			p2 := &Packet{}
			for i := 0; i < b.N; i++ {
				p.Packed = false
				if err := p.Pack(); err != nil {
					b.Fatalf("Pack failed: %v", err)
				}

				p2.Raw = p.Raw
				if err := p2.Unpack(); err != nil {
					b.Fatalf("Unpack failed: %v", err)
				}
			}
		})
	}
}

func BenchmarkPacketPackWarm(b *testing.B) {
	data := make([]byte, 256)
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: make([]byte, 16),
		Data:            data,
	}
	if err := p.Pack(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.Packed = false
		if err := p.Pack(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPacketHashingScale(b *testing.B) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: make([]byte, 16),
		Data:            make([]byte, 256),
	}
	_ = p.Pack()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = p.GetHash()
	}
}

func TestPackHeaderType2RequiresTransportID(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("x"),
	}
	if err := p.Pack(); err == nil {
		t.Fatal("expected error when header type 2 has no transport ID")
	}
}

func TestPackReusesRawBufferWhenCapacitySufficient(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("small"),
		Raw:             make([]byte, 0, MTU),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	capBefore := cap(p.Raw)
	p.Data = append(p.Data, []byte("more")...)
	p.Packed = false
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	if cap(p.Raw) != capBefore {
		t.Fatalf("expected Raw capacity reuse, got cap %d want %d", cap(p.Raw), capBefore)
	}
}

func TestUnpackHeaderType2ExactBoundary(t *testing.T) {
	dest := randomBytes(16)
	tid := randomBytes(16)
	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportID:     tid,
		DestinationHash: dest,
		Context:         ContextNone,
		Data:            []byte("ok"),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}

	short := append([]byte(nil), p.Raw[:2*TruncatedHashLength+MinPacketSize-1]...)
	if err := (&Packet{Raw: short}).Unpack(); err == nil {
		t.Fatal("expected unpack error for short header type 2 packet")
	}

	ht1Short := append([]byte{0x00, 0x00}, dest[:TruncatedHashLength-1]...)
	if err := (&Packet{Raw: ht1Short}).Unpack(); err == nil {
		t.Fatal("expected unpack error for short header type 1 packet")
	}
}

func TestHashablePreimageLenWithShortRaw(t *testing.T) {
	ht1 := &Packet{HeaderType: HeaderType1, Raw: []byte{0x00, 0x01}}
	if n := ht1.hashablePreimageLen(); n != 1 {
		t.Fatalf("ht1 short preimage len=%d want 1", n)
	}

	ht2 := &Packet{HeaderType: HeaderType2, Raw: make([]byte, TruncatedHashLength+2)}
	if n := ht2.hashablePreimageLen(); n != 1 {
		t.Fatalf("ht2 short preimage len=%d want 1", n)
	}

	ht1.Raw = append([]byte{0x00, 0x01}, randomBytes(8)...)
	if n := ht1.hashablePreimageLen(); n != 1+8 {
		t.Fatalf("ht1 preimage len=%d want %d", n, 1+8)
	}

	p := &Packet{
		HeaderType:      HeaderType2,
		PacketType:      PacketTypeData,
		TransportID:     randomBytes(16),
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            randomBytes(32),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	n := p.hashablePreimageLen()
	if n <= 1 {
		t.Fatalf("packed ht2 preimage len=%d", n)
	}
	buf := make([]byte, 0, n)
	hb := p.hashableInto(buf)
	if len(hb) != n {
		t.Fatalf("hashable bytes=%d want %d", len(hb), n)
	}
}

func TestNextRawWireCap(t *testing.T) {
	if got := nextRawWireCap(MTU + 1); got != MTU+1 {
		t.Fatalf("large need cap=%d want %d", got, MTU+1)
	}
	if got := nextRawWireCap(100); got != 128 {
		t.Fatalf("aligned cap=%d want 128", got)
	}
	if got := nextRawWireCap(MTU); got != MTU {
		t.Fatalf("mtu cap=%d want %d", got, MTU)
	}
}

func TestUpdateHashLargePreimage(t *testing.T) {
	p := &Packet{
		HeaderType: HeaderType1,
		Raw:        make([]byte, MTU+50),
	}
	p.Raw[0] = 0x00
	p.Raw[1] = 0x00
	for i := 2; i < len(p.Raw); i++ {
		p.Raw[i] = byte(i)
	}
	p.updateHash()
	if len(p.PacketHash) == 0 {
		t.Fatal("expected hash for large preimage")
	}
}

func TestTruncatedHashLength(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("hash"),
	}
	if err := p.Pack(); err != nil {
		t.Fatal(err)
	}
	th := p.TruncatedHash()
	if len(th) != TruncatedHashLength {
		t.Fatalf("truncated hash len=%d want %d", len(th), TruncatedHashLength)
	}
	if !bytes.Equal(th, p.GetHash()[:TruncatedHashLength]) {
		t.Fatal("truncated hash mismatch")
	}
}

func TestSerializePacksWhenNeeded(t *testing.T) {
	p := &Packet{
		HeaderType:      HeaderType1,
		PacketType:      PacketTypeData,
		DestinationHash: randomBytes(16),
		Context:         ContextNone,
		Data:            []byte("serialize"),
	}
	raw, err := p.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty serialize output")
	}
	if !bytes.Equal(p.DestinationHash, p.Addresses) {
		t.Fatal("Serialize should set Addresses to destination hash")
	}
}

func TestNewAnnouncePacketAppDataFormats(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest := id.Hash()
	transport := randomBytes(16)

	nodePkt, err := NewAnnouncePacket(dest, id, []byte{0x93, 0x01, 0x02, 0x03}, transport)
	if err != nil {
		t.Fatalf("node announce: %v", err)
	}
	if nodePkt.PacketType != PacketTypeAnnounce || nodePkt.HeaderType != HeaderType2 {
		t.Fatalf("unexpected node packet type/header: %d/%d", nodePkt.PacketType, nodePkt.HeaderType)
	}

	name := []byte("peer.one")
	peerApp := append([]byte{0x92, 0xc4, byte(len(name))}, name...)
	peerPkt, err := NewAnnouncePacket(dest, id, peerApp, transport)
	if err != nil {
		t.Fatalf("peer announce: %v", err)
	}
	if len(peerPkt.Data) == 0 {
		t.Fatal("peer announce has empty data")
	}

	fallbackPkt, err := NewAnnouncePacket(dest, id, []byte{0x01}, transport)
	if err != nil {
		t.Fatalf("fallback announce: %v", err)
	}
	if len(fallbackPkt.Data) == 0 {
		t.Fatal("fallback announce has empty data")
	}

	truncatedNameApp := []byte{0x92, 0xc4, 20, 'x'}
	truncPkt, err := NewAnnouncePacket(dest, id, truncatedNameApp, transport)
	if err != nil {
		t.Fatalf("truncated-name announce: %v", err)
	}
	if len(truncPkt.Data) == 0 {
		t.Fatal("truncated-name announce has empty data")
	}

	shortNodeApp := []byte{0x93, 0x01}
	shortPkt, err := NewAnnouncePacket(dest, id, shortNodeApp, transport)
	if err != nil {
		t.Fatalf("short node-prefix announce: %v", err)
	}
	if len(shortPkt.Data) == 0 {
		t.Fatal("short node-prefix announce has empty data")
	}

	exactPeerApp := []byte{0x92, 0xc4, 1, 'z'}
	exactPkt, err := NewAnnouncePacket(dest, id, exactPeerApp, transport)
	if err != nil {
		t.Fatalf("exact peer announce: %v", err)
	}
	if len(exactPkt.Data) == 0 {
		t.Fatal("exact peer announce has empty data")
	}
}

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
