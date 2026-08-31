// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"testing"
)

func FuzzRNodeDecoder(f *testing.F) {
	f.Add([]byte{KISSFend, rnodeCmdData, 1, 2, KISSFend})
	f.Add([]byte{KISSFend, rnodeCmdData, KISSFesc, KISSTFend, KISSFend})
	f.Fuzz(func(t *testing.T, data []byte) {
		decoder := newRNodeCmdDecoder(rnodeHWMTU, func(byte, []byte) {})
		decoder.feed(data)
	})
}

func FuzzRNodeEscapeRoundTrip(f *testing.F) {
	f.Add([]byte{1, KISSFend, 2, KISSFesc, 3})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > rnodeHWMTU {
			return
		}
		var got []byte
		decoder := newRNodeCmdDecoder(rnodeHWMTU, func(cmd byte, data []byte) {
			if cmd == rnodeCmdData {
				got = append([]byte(nil), data...)
			}
		})
		decoder.feed(appendRNodeDataFrame(nil, payload))
		if !bytes.Equal(got, payload) {
			t.Fatalf("round trip got %x, want %x", got, payload)
		}
	})
}
