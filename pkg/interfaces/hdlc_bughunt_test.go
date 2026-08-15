// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"
)

func TestBughuntHDLCRejectsOversizePayload(t *testing.T) {
	var got []byte
	d := newHDLCToggleStreamDecoder(32, func(p []byte) {
		got = append([]byte(nil), p...)
	})
	frame := []byte{HDLCFlag}
	for i := range 64 {
		frame = append(frame, byte(i))
	}
	frame = append(frame, HDLCFlag)
	d.feed(frame)
	if got != nil {
		t.Fatalf("delivered %d-byte payload over MTU 32", len(got))
	}
}

func TestBughuntHDLCDeliversWithinMTU(t *testing.T) {
	var got []byte
	d := newHDLCToggleStreamDecoder(32, func(p []byte) {
		got = append([]byte(nil), p...)
	})
	payload := make([]byte, 20)
	for i := range payload {
		payload[i] = byte(i + 1)
	}
	frame := append([]byte{HDLCFlag}, payload...)
	frame = append(frame, HDLCFlag)
	d.feed(frame)
	if len(got) != 20 {
		t.Fatalf("got len=%d want 20", len(got))
	}
}
