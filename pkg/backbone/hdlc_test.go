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

func TestAppendFrameHDLCEqualsFrameHDLC(t *testing.T) {
	payloads := [][]byte{
		bytes.Repeat([]byte{0x01}, 20),
		{0x42, hdlcFlag, hdlcEsc, 0x00},
		bytes.Repeat([]byte{0x7E, 0x7D, 0x01}, 40),
	}
	for i, p := range payloads {
		got := appendFrameHDLC(nil, p)
		want := frameHDLC(p)
		if !bytes.Equal(got, want) {
			t.Fatalf("case %d: append != frame\n got=%x\nwant=%x", i, got, want)
		}
		reuse := make([]byte, 8)
		got = appendFrameHDLC(reuse[:0], p)
		if !bytes.Equal(got, want) {
			t.Fatalf("case %d reuse: append != frame", i)
		}
	}
}

func TestHDLCDecoderBatchManyFrames(t *testing.T) {
	const n = 64
	var got [][]byte
	d := NewHDLCDecoder(4096, func(pkt []byte) {
		got = append(got, append([]byte(nil), pkt...))
	})
	var blob []byte
	want := make([][]byte, n)
	for i := range n {
		p := bytes.Repeat([]byte{byte(i + 1)}, 24)
		p[0], p[1] = hdlcFlag, hdlcEsc
		want[i] = p
		blob = append(blob, frameHDLC(p)...)
	}
	d.Feed(blob)
	if len(got) != n {
		t.Fatalf("got %d frames want %d", len(got), n)
	}
	for i := range n {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("frame %d mismatch", i)
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
	payload := bytes.Repeat([]byte{0x01, 0x7E, 0x7D, 0xFF}, 6) // 24 bytes > HEADER_MINSIZE
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
		bytes.Repeat([]byte{0x01}, 20),
		bytes.Repeat([]byte{0x02}, 24),
		bytes.Repeat([]byte{0x03}, 32),
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

func TestHDLCDecoderDropsBelowHeaderMinSize(t *testing.T) {
	var got int
	d := NewHDLCDecoder(4096, func([]byte) { got++ })
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x01}, 19)))
	if got != 0 {
		t.Fatalf("expected drop of HEADER_MINSIZE frame, got=%d", got)
	}
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x01}, 20)))
	if got != 1 {
		t.Fatalf("got=%d want 1", got)
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
	d.Feed(frameHDLC(bytes.Repeat([]byte{0x02}, 20)))
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
	f.Add(bytes.Repeat([]byte{0x42}, 20))
	f.Add(append([]byte{hdlcFlag, 0x00}, bytes.Repeat([]byte{0x01}, 20)...))
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) <= 19 {
			t.Skip("below HEADER_MINSIZE")
		}
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

func TestHDLCAssemblerCapVsLargeMTU(t *testing.T) {
	d := NewHDLCDecoder(1<<20, func([]byte) {})
	if cap(d.data) > streamReadChunk {
		t.Fatalf("assembler cap=%d want <= %d for 1MiB MTU", cap(d.data), streamReadChunk)
	}
	if d.mtu != 1<<20 {
		t.Fatalf("wire MTU=%d want %d", d.mtu, 1<<20)
	}
	payload := bytes.Repeat([]byte{0x01}, 20)
	var got []byte
	d.onPacket = func(pkt []byte) { got = append([]byte(nil), pkt...) }
	d.Feed(frameHDLC(payload))
	if !bytes.Equal(got, payload) {
		t.Fatalf("small frame under large MTU: %x != %x", got, payload)
	}
}
