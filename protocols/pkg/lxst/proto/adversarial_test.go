// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestUnpackEmpty(t *testing.T) {
	if _, err := proto.Unpack(nil); err != proto.ErrEmptyPacket {
		t.Fatalf("nil: %v", err)
	}
	if _, err := proto.Unpack([]byte{}); err != proto.ErrEmptyPacket {
		t.Fatalf("empty: %v", err)
	}
}

func TestPackSignallingEmpty(t *testing.T) {
	if _, err := proto.PackSignalling(nil); err != proto.ErrEmptyPacket {
		t.Fatalf("nil signals: %v", err)
	}
	if _, err := proto.PackSignalling([]int{}); err != proto.ErrEmptyPacket {
		t.Fatalf("empty signals: %v", err)
	}
}

func TestUnpackRejectsEmptyMap(t *testing.T) {
	if _, err := proto.Unpack([]byte{0x80}); err != proto.ErrMissingFields {
		t.Fatalf("empty map: %v", err)
	}
}

func TestUnpackScalarSignalling(t *testing.T) {
	pkt, err := proto.Unpack([]byte{0x81, 0x00, 0x03})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != 1 || pkt.Signals[0] != proto.StatusAvailable {
		t.Fatalf("scalar signalling %+v", pkt.Signals)
	}
}

func TestUnpackTruncatedMap(t *testing.T) {
	if _, err := proto.Unpack([]byte{0x81, 0x00}); err == nil {
		t.Fatal("truncated map")
	}
}

func TestUnpackIgnoresUnknownKeys(t *testing.T) {
	raw, err := proto.PackSignalling([]int{proto.StatusBusy})
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt.Signals) != 1 || pkt.Signals[0] != proto.StatusBusy {
		t.Fatalf("signals %+v", pkt.Signals)
	}
}

func TestUnpackRejectsOversized(t *testing.T) {
	raw := make([]byte, 4097)
	raw[0] = 0x81
	raw[1] = 0x00
	raw[2] = 0x03
	if _, err := proto.Unpack(raw); err != proto.ErrPacketTooLarge {
		t.Fatalf("got %v", err)
	}
}

func TestUnpackRejectsTooManyMapKeys(t *testing.T) {
	raw := []byte{0x85, 0x00, 0x03, 0x01, 0xc4, 0x01, 0x01, 0x02, 0x01, 0x03, 0x01, 0x04, 0x01}
	if _, err := proto.Unpack(raw); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestUnpackRejectsClaimedHugeArray(t *testing.T) {
	raw := []byte{0x81, 0x00, 0xdd, 0x00, 0x0f, 0x42, 0x40}
	if _, err := proto.Unpack(raw); err == nil {
		t.Fatal("array32 bomb")
	}
}

func FuzzUnpackHostile(f *testing.F) {
	f.Add([]byte{0x81, 0x00, 0x03})
	f.Add([]byte{0xde, 0xff, 0xff})
	f.Add([]byte{0xdd, 0xff, 0xff, 0xff, 0xff})
	f.Add(bytes.Repeat([]byte{0x80}, 16))
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = proto.Unpack(raw)
	})
}

func TestUnpackMalformed(t *testing.T) {
	cases := [][]byte{
		{0xff},
		{0x91, 0x01},
		{0x81, 0xa1, 'x', 0x01},
		{0x81, 0x00, 0xa3, 'n', 'o', 'p'},
		{0x81, 0x01, 0x01},
		bytes.Repeat([]byte{0xdf}, 8),
	}
	for i, raw := range cases {
		_, err := proto.Unpack(raw)
		if err == nil {
			t.Fatalf("case %d unpacked", i)
		}
	}
}

func TestSplitFrameTooShort(t *testing.T) {
	if _, _, err := proto.SplitFrame(nil); err == nil {
		t.Fatal("nil frame")
	}
	codec, payload, err := proto.SplitFrame([]byte{proto.CodecOpus})
	if err != nil {
		t.Fatal(err)
	}
	if codec != proto.CodecOpus || len(payload) != 0 {
		t.Fatalf("codec %d payload %d", codec, len(payload))
	}
}

func TestCodec2FrameRoundTrip(t *testing.T) {
	payload := []byte{0x06, 0xaa, 0xbb}
	raw, err := proto.PackFrame(proto.CodecCodec2, payload)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := proto.Unpack(raw)
	if err != nil {
		t.Fatal(err)
	}
	codec, body, err := proto.SplitFrame(pkt.Frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if codec != proto.CodecCodec2 || !bytes.Equal(body, payload) {
		t.Fatalf("codec %d body %x", codec, body)
	}
}

func TestPreferredModeBounds(t *testing.T) {
	if proto.IsPreferredMode(proto.PreferredMode - 1) {
		t.Fatal("below mode range")
	}
	if !proto.IsPreferredMode(proto.SignalPreferredMode(proto.ModeFullDuplex)) {
		t.Fatal("full duplex mode")
	}
	if proto.IsPreferredMode(proto.PreferredProfile) {
		t.Fatal("profile is not mode")
	}
	if proto.IsPreferredProfile(proto.PreferredProfile - 1) {
		t.Fatal("below profile range")
	}
}

func TestIsAutoStatus(t *testing.T) {
	if proto.IsAutoStatus(proto.StatusBusy) || proto.IsAutoStatus(proto.StatusRejected) {
		t.Fatal("busy/rejected are not auto")
	}
	if !proto.IsAutoStatus(proto.StatusAvailable) || !proto.IsAutoStatus(proto.StatusEstablished) {
		t.Fatal("available/established are auto")
	}
}

func FuzzPackUnpackSignalling(f *testing.F) {
	f.Add(3, 4, 319)
	f.Fuzz(func(t *testing.T, a, b, c int) {
		raw, err := proto.PackSignalling([]int{a, b, c})
		if err != nil {
			t.Fatal(err)
		}
		pkt, err := proto.Unpack(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(pkt.Signals) != 3 || pkt.Signals[0] != a || pkt.Signals[1] != b || pkt.Signals[2] != c {
			t.Fatalf("round trip %+v", pkt.Signals)
		}
	})
}

func FuzzPackUnpackFrame(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 2048 {
			payload = payload[:2048]
		}
		raw, err := proto.PackFrame(proto.CodecOpus, payload)
		if err != nil {
			t.Fatal(err)
		}
		pkt, err := proto.Unpack(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(pkt.Frames) != 1 {
			if len(payload) == 0 {
				return
			}
			t.Fatalf("frames %d", len(pkt.Frames))
		}
		codec, body, err := proto.SplitFrame(pkt.Frames[0])
		if err != nil {
			t.Fatal(err)
		}
		if codec != proto.CodecOpus || !bytes.Equal(body, payload) {
			t.Fatalf("codec %d body mismatch", codec)
		}
	})
}
