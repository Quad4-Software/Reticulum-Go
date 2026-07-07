// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

func FuzzSplitPipeCommand(f *testing.F) {
	f.Add("cat")
	f.Add("netcat -l 5757")
	f.Add(`echo "hello"`)
	f.Fuzz(func(t *testing.T, cmd string) {
		args, err := splitPipeCommand(cmd)
		if err != nil {
			return
		}
		if len(args) == 0 {
			t.Fatal("empty args from successful split")
		}
		for _, a := range args {
			if a == "" {
				t.Fatal("empty argument")
			}
		}
	})
}

func FuzzHDLCStreamDecoder(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{0x7e, 0x7d, 0x5e})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > pipeHWMTU {
			data = data[:pipeHWMTU]
		}
		frame := appendFrameHDLC(nil, data)
		var got []byte
		d := newHDLCStreamDecoder(pipeHWMTU, func(payload []byte) {
			got = append([]byte(nil), payload...)
		})
		d.feed(frame)
		if len(data) < 2 {
			return
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("round-trip failed: in=%x out=%x", data, got)
		}
	})
}

func FuzzPipeHDLCFrameDecode(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{0x7e, 0x7d, 0x5e})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > pipeHWMTU {
			data = data[:pipeHWMTU]
		}
		decoded := unescapeHDLC(escapeHDLC(data))
		if !bytes.Equal(decoded, data) {
			t.Fatalf("round-trip failed: in=%x out=%x", data, decoded)
		}
	})
}
