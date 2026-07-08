// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package backbone

import (
	"bytes"
	"testing"
)

func TestEscapeHDLCRoundTrip(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x01, 0x02, 0x03},
		{hdlcFlag, hdlcEsc, 0xAB},
		bytes.Repeat([]byte{0x7E, 0x7D}, 64),
		make([]byte, 2048),
	}
	for i, in := range cases {
		if len(in) > 0 {
			for j := range in {
				in[j] = byte(i + j)
			}
		}
		out := unescapeHDLC(escapeHDLC(in))
		if !bytes.Equal(in, out) {
			t.Fatalf("case %d: in=%x out=%x", i, in, out)
		}
	}
}

func TestFrameHDLCBoundaries(t *testing.T) {
	payload := []byte{0x42, hdlcFlag, hdlcEsc, 0x00}
	frame := frameHDLC(payload)
	if frame[0] != hdlcFlag || frame[len(frame)-1] != hdlcFlag {
		t.Fatalf("missing frame flags: %x", frame)
	}
	for i := 1; i < len(frame)-1; i++ {
		if frame[i] == hdlcFlag {
			t.Fatalf("raw flag inside frame at %d", i)
		}
	}
}

func TestHDLCDecoderSinglePacket(t *testing.T) {
	var got []byte
	d := NewHDLCDecoder(4096, func(pkt []byte) {
		got = append([]byte(nil), pkt...)
	})
	payload := []byte{0x01, 0x7E, 0x7D, 0xFF}
	d.Feed(frameHDLC(payload))
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %x want %x", got, payload)
	}
}

func TestHDLCDecoderSplitDelivery(t *testing.T) {
	var packets [][]byte
	d := NewHDLCDecoder(4096, func(pkt []byte) {
		packets = append(packets, append([]byte(nil), pkt...))
	})
	frames := [][]byte{
		{0x01, 0x02},
		{0x03, 0x04},
		{0x05},
	}
	for _, payload := range frames {
		frame := frameHDLC(payload)
		mid := len(frame) / 2
		d.Feed(frame[:mid])
		d.Feed(frame[mid:])
	}
	if len(packets) != len(frames) {
		t.Fatalf("packets=%d want %d", len(packets), len(frames))
	}
	for i := range frames {
		if !bytes.Equal(packets[i], frames[i]) {
			t.Fatalf("packet %d: %x != %x", i, packets[i], frames[i])
		}
	}
}

func TestHDLCDecoderOversizeDropped(t *testing.T) {
	var got []byte
	d := NewHDLCDecoder(200, func(pkt []byte) {
		got = pkt
	})
	big := bytes.Repeat([]byte{0xAA}, 500)
	d.Feed(frameHDLC(big))
	if len(got) != 0 {
		t.Fatalf("expected oversize frame drop, got len %d", len(got))
	}
}

func TestHDLCDecoderReset(t *testing.T) {
	var got int
	d := NewHDLCDecoder(4096, func([]byte) { got++ })
	d.Feed([]byte{hdlcFlag, 0x01})
	d.Reset()
	d.Feed(frameHDLC([]byte{0x02}))
	if got != 1 {
		t.Fatalf("got=%d want 1", got)
	}
}

func FuzzHDLCEscapeRoundTrip(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{hdlcFlag, hdlcEsc, 0xAB})
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0x7D}, 128))
	f.Fuzz(func(t *testing.T, data []byte) {
		out := unescapeHDLC(escapeHDLC(data))
		if !bytes.Equal(out, data) {
			t.Fatalf("round-trip failed")
		}
	})
}

func FuzzHDLCDecoderFeed(f *testing.F) {
	f.Add([]byte{0x7E, 0x01, 0x02, 0x7E})
	f.Add([]byte{0x7E, 0x7D, 0x5E, 0x7E})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		var packets int
		d := NewHDLCDecoder(4096, func([]byte) { packets++ })
		d.Feed(data)
		if packets < 0 {
			t.Fatal("negative packet count")
		}
	})
}

func FuzzFrameHDLCDecode(f *testing.F) {
	f.Add([]byte{0x42, 0x43})
	f.Add([]byte{hdlcFlag, 0x00})
	f.Fuzz(func(t *testing.T, payload []byte) {
		frame := frameHDLC(payload)
		var got []byte
		d := NewHDLCDecoder(len(payload)+64, func(pkt []byte) {
			got = append([]byte(nil), pkt...)
		})
		d.Feed(frame)
		if !bytes.Equal(got, payload) {
			t.Fatalf("decode mismatch")
		}
	})
}
