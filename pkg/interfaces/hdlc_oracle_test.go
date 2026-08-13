// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"testing"
)

// Guarantee: TCP HDLC framing matches flag + escape + flag and never emits
// an unescaped 0x7E inside a frame.
func TestOracle_TCPHDLCEscapeTable(t *testing.T) {
	payload := []byte{0x00, HDLCFlag, HDLCEsc, 0x7F, 0x01}
	frame := appendFrameHDLC(nil, payload)
	legacy := append([]byte{HDLCFlag}, escapeHDLC(payload)...)
	legacy = append(legacy, HDLCFlag)
	if !bytes.Equal(frame, legacy) {
		t.Fatalf("appendFrameHDLC diverged from Python-style flag+escape+flag")
	}
	for i := 1; i < len(frame)-1; i++ {
		if frame[i] == HDLCFlag {
			t.Fatalf("unescaped interior flag at %d", i)
		}
	}
}
