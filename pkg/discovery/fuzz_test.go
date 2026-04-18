// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package discovery

import (
	"bytes"
	"testing"
)

// FuzzDecodeAppData ensures the app_data parser is panic-free for arbitrary
// inputs. It does not assert successful decoding; bad inputs must surface as
// a regular error.
func FuzzDecodeAppData(f *testing.F) {
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0xff}, StampSize+1))
	f.Add(bytes.Repeat([]byte{0x00}, 1024))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		_, _, _, err := DecodeAppData(raw)
		_ = err
	})
}

// FuzzDecodeInfo confirms the msgpack info parser survives arbitrary inputs.
func FuzzDecodeInfo(f *testing.F) {
	good, err := EncodeInfo(Info{
		Type:        "TCPInterface",
		Transport:   true,
		TransportID: bytes.Repeat([]byte{0xab}, 16),
		Name:        "fuzz",
		Latitude:    1.0,
		Longitude:   2.0,
		Height:      3.0,
	})
	if err == nil {
		f.Add(good)
	}
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Add(bytes.Repeat([]byte{0xff}, 1024))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		_, err := DecodeInfo(raw)
		_ = err
	})
}

// FuzzStampValid ensures StampValid handles arbitrary stamp/workblock
// inputs without panicking, regardless of length or the supplied target
// cost.
func FuzzStampValid(f *testing.F) {
	f.Add([]byte{}, []byte{}, 0)
	f.Add(bytes.Repeat([]byte{0x00}, StampSize), bytes.Repeat([]byte{0x00}, 256), 256)
	f.Add(bytes.Repeat([]byte{0xff}, StampSize), bytes.Repeat([]byte{0xff}, 256), -1)
	f.Add(bytes.Repeat([]byte{0xaa}, 16), bytes.Repeat([]byte{0xbb}, 64), 9999)

	f.Fuzz(func(t *testing.T, stamp, workblock []byte, target int) {
		if len(stamp) > 1<<10 || len(workblock) > 1<<14 {
			t.Skip()
		}
		_ = StampValid(stamp, target, workblock)
	})
}
