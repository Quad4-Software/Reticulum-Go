// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRecvFramedRejectsOversizeBeforeAlloc(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 1<<30)
	_, err := RecvFramed(bytes.NewReader(hdr[:]), 64)
	if err == nil {
		t.Fatal("expected oversize error")
	}
}

func TestRecvFramedRejectsNegativeSize(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(0x80000000))
	_, err := RecvFramed(bytes.NewReader(hdr[:]), 64)
	if err == nil {
		t.Fatal("expected negative size error")
	}
}

func TestRecvFramedRejectsExtendedOversize(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, int32(-1))
	_ = binary.Write(&buf, binary.BigEndian, uint64(1<<40))
	_, err := RecvFramed(&buf, 1024)
	if err == nil {
		t.Fatal("expected extended oversize error")
	}
}

func TestSendRecvFramedRoundTrip(t *testing.T) {
	payloads := [][]byte{nil, {}, []byte("x"), bytes.Repeat([]byte{0xab}, 300)}
	for _, p := range payloads {
		var buf bytes.Buffer
		if err := SendFramed(&buf, p); err != nil {
			t.Fatalf("SendFramed: %v", err)
		}
		got, err := RecvFramed(&buf, 1<<20)
		if err != nil {
			t.Fatalf("RecvFramed: %v", err)
		}
		if !bytes.Equal(got, p) {
			t.Fatalf("got %q want %q", got, p)
		}
	}
}

func TestParseDigestOracle(t *testing.T) {
	name, payload := parseDigest([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if name != "" || len(payload) != 16 {
		t.Fatalf("md5-sized: name=%q len=%d", name, len(payload))
	}
	name, payload = parseDigest([]byte("{sha256}abcdefghijklmnopabcdefghijklmnop"))
	if name != "sha256" || len(payload) == 0 {
		t.Fatalf("sha256: name=%q payload=%q", name, payload)
	}
	name, _ = parseDigest([]byte("{unknown}xx"))
	if name != "" {
		t.Fatalf("unknown digest must be ignored, got %q", name)
	}
	name, payload = parseDigest([]byte("not-a-digest"))
	if name != "" || !bytes.Equal(payload, []byte("not-a-digest")) {
		t.Fatalf("plain: name=%q payload=%q", name, payload)
	}
}

// FuzzRecvFramedOracle feeds arbitrary length-prefixed frames. With a
// positive maxSize the reader must never return a buffer larger than maxSize.
func FuzzRecvFramedOracle(f *testing.F) {
	var good bytes.Buffer
	_ = SendFramed(&good, []byte("ok"))
	f.Add(good.Bytes(), 64)
	f.Add([]byte{0, 0, 0, 5, 'h', 'e', 'l', 'l', 'o'}, 5)
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, 8)
	f.Add([]byte{0x80, 0, 0, 0}, 8)
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 1}, 1)

	f.Fuzz(func(t *testing.T, raw []byte, maxSize int) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		if maxSize < 0 {
			maxSize = -maxSize
		}
		if maxSize > 1<<20 {
			maxSize = 1 << 20
		}
		if maxSize == 0 {
			maxSize = 256
		}
		got, err := RecvFramed(bytes.NewReader(raw), maxSize)
		if err != nil {
			return
		}
		if len(got) > maxSize {
			t.Fatalf("len=%d exceeds maxSize=%d", len(got), maxSize)
		}
	})
}

func TestDecodeHashOracle(t *testing.T) {
	if decodeHash(nil) != nil {
		t.Fatal("nil should decode to nil")
	}
	if decodeHash(123) != nil {
		t.Fatal("int should decode to nil")
	}
	if decodeHash("zz") != nil {
		t.Fatal("bad hex should decode to nil")
	}
	b := decodeHash("0011223344556677")
	if len(b) != 8 {
		t.Fatalf("hex len=%d", len(b))
	}
	raw := []byte{1, 2, 3}
	if !bytes.Equal(decodeHash(raw), raw) {
		t.Fatal("[]byte passthrough failed")
	}
}
