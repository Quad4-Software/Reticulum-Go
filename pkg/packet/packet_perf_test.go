// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package packet

import (
	"fmt"
	"testing"
)

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
