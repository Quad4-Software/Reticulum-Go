// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package packet

import (
	"bytes"
	"testing"
)

func oracleHT1(hops byte, dest []byte, data []byte) []byte {
	raw := make([]byte, 0, MinPacketSize+TruncatedHashLength+len(data))
	raw = append(raw, 0x00, hops)
	raw = append(raw, dest...)
	raw = append(raw, ContextNone)
	return append(raw, data...)
}

func oracleHT2(hops byte, tid, dest, data []byte) []byte {
	flags := byte(HeaderType2 << 6)
	raw := make([]byte, 0, MinPacketSize+2*TruncatedHashLength+len(data))
	raw = append(raw, flags, hops)
	raw = append(raw, tid...)
	raw = append(raw, dest...)
	raw = append(raw, ContextNone)
	return append(raw, data...)
}

// Guarantee: Unpack rejects hop counts at or above PATHFINDER_M and accepts 127.
func TestOracle_UnpackRejectsPathfinderMAccepts127(t *testing.T) {
	dest := bytes.Repeat([]byte{0x11}, TruncatedHashLength)
	p := &Packet{Raw: oracleHT1(PathfinderM-1, dest, []byte("ok"))}
	if err := p.Unpack(); err != nil {
		t.Fatalf("hops=127: %v", err)
	}
	for _, hops := range []byte{PathfinderM, 200, 255} {
		p := &Packet{Raw: oracleHT1(hops, dest, []byte("no"))}
		if err := p.Unpack(); err == nil {
			t.Fatalf("hops=%d must reject", hops)
		}
	}
}

// Guarantee: truncated header-type-2 frames never unpack.
func TestOracle_UnpackRejectsTruncatedHT2(t *testing.T) {
	tid := bytes.Repeat([]byte{0x22}, TruncatedHashLength)
	dest := bytes.Repeat([]byte{0x33}, TruncatedHashLength)
	full := oracleHT2(1, tid, dest, []byte("x"))
	need := 2*TruncatedHashLength + MinPacketSize
	if len(full) < need {
		t.Fatalf("fixture short: %d", len(full))
	}
	p := &Packet{Raw: full[:need-1]}
	if err := p.Unpack(); err == nil {
		t.Fatal("truncated HT2 unpacked")
	}
	p = &Packet{Raw: full}
	if err := p.Unpack(); err != nil {
		t.Fatalf("valid HT2: %v", err)
	}
}

// Guarantee: oversize inbound frames are refused.
func TestOracle_UnpackRejectsOversize(t *testing.T) {
	p := &Packet{Raw: make([]byte, MaxInboundPacketSize+1)}
	if err := p.Unpack(); err == nil {
		t.Fatal("oversize unpacked")
	}
}

// Guarantee: packets shorter than MinPacketSize never unpack.
func TestOracle_UnpackRejectsTooShort(t *testing.T) {
	p := &Packet{Raw: []byte{0x00, 0x00}}
	if err := p.Unpack(); err == nil {
		t.Fatal("2-byte packet unpacked")
	}
}
