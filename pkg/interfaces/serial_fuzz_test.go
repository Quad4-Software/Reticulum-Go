// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"testing"
	"time"
)

func FuzzSerialHDLCRoundTrip(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{0x7e, 0x7d, 0x5e})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > serialHWMTU {
			data = data[:serialHWMTU]
		}
		frame := appendFrameHDLC(nil, data)
		var got []byte
		d := newHDLCStreamDecoder(serialHWMTU, func(payload []byte) {
			got = append([]byte(nil), payload...)
		})
		if len(data) == 0 {
			d.feed(frame)
			return
		}
		// Feed in irregular chunks to stress the assembler.
		for i := 0; i < len(frame); {
			n := 1 + int(data[0])%7
			if i == 0 {
				n = 1 + int(data[0])%5
			}
			if n < 1 {
				n = 1
			}
			end := min(i+n, len(frame))
			d.feed(frame[i:end])
			i = end
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip failed in=%x out=%x", data, got)
		}
	})
}

func FuzzSerialHDLCIdleDrop(f *testing.F) {
	f.Add([]byte{0x7e, 0x01, 0x02})
	f.Fuzz(func(t *testing.T, partial []byte) {
		d := newHDLCStreamDecoder(serialHWMTU, func([]byte) {})
		d.feed(partial)
		_ = d.dropPartial()
		// Feeding a complete frame after drop must still work.
		payload := []byte{0xaa, 0xbb}
		frame := appendFrameHDLC(nil, payload)
		var got []byte
		d2 := newHDLCStreamDecoder(serialHWMTU, func(p []byte) {
			got = append([]byte(nil), p...)
		})
		d2.feed(partial)
		_ = d2.dropPartial()
		d2.feed(frame)
		if !bytes.Equal(got, payload) {
			t.Fatalf("after idle drop got %x", got)
		}
		_ = time.Millisecond
	})
}
