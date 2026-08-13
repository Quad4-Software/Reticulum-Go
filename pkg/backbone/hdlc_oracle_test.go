// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"bytes"
	"testing"
)

// Guarantee: framed payloads never contain an unescaped 0x7E between flags.
func TestOracle_FrameHDLCNeverEmitsInteriorFlag(t *testing.T) {
	payloads := [][]byte{
		bytes.Repeat([]byte{0x01}, 20),
		{hdlcFlag, hdlcEsc, 0x00, 0x01},
		bytes.Repeat([]byte{hdlcFlag}, 40),
		bytes.Repeat([]byte{hdlcEsc}, 40),
	}
	for i, p := range payloads {
		frame := appendFrameHDLC(nil, p)
		if frame[0] != hdlcFlag || frame[len(frame)-1] != hdlcFlag {
			t.Fatalf("case %d missing flags", i)
		}
		for j := 1; j < len(frame)-1; j++ {
			if frame[j] == hdlcFlag {
				t.Fatalf("case %d unescaped flag at %d", i, j)
			}
		}
		var got []byte
		d := NewHDLCDecoder(len(p)+64, func(pkt []byte) {
			got = append([]byte(nil), pkt...)
		})
		d.Feed(frame)
		if len(p) > 19 && !bytes.Equal(got, p) {
			t.Fatalf("case %d round-trip mismatch", i)
		}
	}
}

// Guarantee: HEADER_MINSIZE frames are dropped and MTU oversize is dropped.
func TestOracle_HDLCDecoderDropsInvalidFrameLen(t *testing.T) {
	var n int
	d := NewHDLCDecoder(32, func([]byte) { n++ })
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x01}, 19)))
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x01}, 40)))
	if n != 0 {
		t.Fatalf("invalid frames delivered n=%d", n)
	}
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x01}, 20)))
	if n != 1 {
		t.Fatalf("valid 20-byte payload dropped n=%d", n)
	}
}

func TestOracle_BackboneHDLCCallbackMustCopy(t *testing.T) {
	var aliased []byte
	d := NewHDLCDecoder(4096, func(pkt []byte) {
		aliased = pkt
	})
	first := bytes.Repeat([]byte{0x01}, 24)
	d.Feed(frameHDLC(first))
	if aliased == nil {
		t.Fatal("first frame not delivered")
	}
	held := aliased
	second := bytes.Repeat([]byte{0x02}, 24)
	d.Feed(frameHDLC(second))
	if bytes.Equal(held, first) {
		t.Fatal("callback slice was copied, assembler reuse contract changed")
	}
}
