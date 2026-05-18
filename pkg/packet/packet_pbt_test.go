// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package packet

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/Quad4-Software/pbt/pkg/pbt"
)

func randomHash16(r *rand.Rand) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

func genValidPacket(r *rand.Rand, size int) *Packet {
	headerType := byte(r.Intn(2))
	packetType := byte(r.Intn(4))
	transportType := byte(r.Intn(2))
	context := byte(r.Intn(256))
	contextFlag := byte(r.Intn(2))
	hops := byte(r.Intn(256))
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
