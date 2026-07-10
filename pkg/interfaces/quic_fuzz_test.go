// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func FuzzParsePeerKeyPin(f *testing.F) {
	f.Add("")
	f.Add("aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899")
	f.Add("not-hex")
	f.Add(hex.EncodeToString(make([]byte, sha256.Size)))
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = parsePeerKeyPin(s)
	})
}

func FuzzHDLCViaQUICDecoder(f *testing.F) {
	f.Add([]byte{HDLCFlag, 0x01, 0x02, HDLCFlag})
	f.Add([]byte{HDLCFlag, HDLCEsc, HDLCFlag ^ HDLCEscMask, HDLCFlag})
	f.Add(make([]byte, 4096))
	f.Fuzz(func(t *testing.T, data []byte) {
		var n int
		d := newHDLCToggleStreamDecoder(DefaultMTU, func(payload []byte) {
			n++
			if len(payload) > DefaultMTU {
				t.Fatalf("payload larger than MTU: %d", len(payload))
			}
		})
		// Feed in chunks to stress incremental decode.
		for i := 0; i < len(data); {
			end := min(i+17, len(data))
			d.feed(data[i:end])
			i = end
		}
		_ = n
	})
}
