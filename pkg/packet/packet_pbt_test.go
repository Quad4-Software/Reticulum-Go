// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
)

// Property suite for packet wire invariants. Pack/unpack round-trip lives in
// packet_test.go (TestPBTPacketPackUnpackRoundTrip).

func TestPBTPackedPacketFitsMTU(t *testing.T) {
	gen := pbt.NewGenerator("validPacket", genValidPacket)
	prop := pbt.ForAll(
		"packed packet length within MTU",
		gen,
		func(p *Packet) bool {
			if err := p.Pack(); err != nil {
				return false
			}
			return len(p.Raw) > 0 && len(p.Raw) <= MTU
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(55), pbt.WithMaxSize(450))
}

func TestPBTPacketHashStableAcrossPack(t *testing.T) {
	gen := pbt.NewGenerator("validPacket", genValidPacket)
	prop := pbt.ForAll(
		"GetHash stable after Pack",
		gen,
		func(p *Packet) bool {
			if err := p.Pack(); err != nil {
				return false
			}
			h1 := p.GetHash()
			h2 := p.GetHash()
			return len(h1) > 0 && bytes.Equal(h1, h2)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(56), pbt.WithMaxSize(450))
}
