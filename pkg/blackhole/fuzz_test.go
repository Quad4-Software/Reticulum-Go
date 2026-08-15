// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package blackhole

import (
	"bytes"
	"testing"
)

// FuzzDecodeBlackholeMap confirms that arbitrary byte input never causes
// the msgpack decoder, allocation, or downstream type assertions to panic.
// Decode errors are expected for arbitrary input and are ignored.
func FuzzDecodeBlackholeMap(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add(bytes.Repeat([]byte{0xff}, 1024))

	if buf, err := encodeBlackholeMap(map[string]map[string]any{
		string(bytes.Repeat([]byte{0x01}, HashLen)): {
			"source": bytes.Repeat([]byte{0x02}, HashLen),
			"until":  float64(0),
			"reason": "fuzz",
		},
	}); err == nil {
		f.Add(buf)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip("oversize input")
		}
		_, err := DecodeBlackholeMap(raw)
		_ = err
	})
}

// FuzzEncodeDecodeRoundTrip exercises the entry encode/decode boundary by
// constructing an entry with arbitrary inputs and confirming the result
// survives a round trip without losing the source/until/reason fields.
func FuzzEncodeDecodeRoundTrip(f *testing.F) {
	f.Add([]byte{0x00, 0x01, 0x02, 0x03}, "alpha", uint64(0))
	f.Add(bytes.Repeat([]byte{0x42}, HashLen), "beta", uint64(1))
	f.Fuzz(func(t *testing.T, sourceSeed []byte, reason string, untilU uint64) {
		if len(sourceSeed) == 0 {
			t.Skip()
		}
		hash := bytes.Repeat([]byte{0xaa}, HashLen)
		source := make([]byte, HashLen)
		for i := 0; i < HashLen && i < len(sourceSeed); i++ {
			source[i] = sourceSeed[i]
		}
		until := float64(untilU % 1_000_000_000)
		entry := Entry{Source: source, Until: until, Reason: reason}
		raw, err := encodeBlackholeMap(map[string]map[string]any{
			string(hash): entryToMsgpackMap(entry),
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := DecodeBlackholeMap(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		v, ok := got[string(hash)]
		if !ok {
			t.Fatalf("missing hash key after round trip")
		}
		if !bytes.Equal(v.Source, source) {
			t.Fatalf("source mismatch: got=%x want=%x", v.Source, source)
		}
		if v.Until != until {
			t.Fatalf("until mismatch: got=%v want=%v", v.Until, until)
		}
		if v.Reason != reason {
			t.Fatalf("reason mismatch: got=%q want=%q", v.Reason, reason)
		}
	})
}
