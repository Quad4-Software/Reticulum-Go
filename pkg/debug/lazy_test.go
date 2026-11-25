// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"bytes"
	"fmt"
	"testing"
)

func TestHexLogValue(t *testing.T) {
	v := Hex([]byte{0x00, 0xab, 0xff}).LogValue()
	if got := v.String(); got != "00abff" {
		t.Fatalf("Hex LogValue = %q, want 00abff", got)
	}
	if got := fmt.Sprint(Hex([]byte{0x01, 0x23})); got != "0123" {
		t.Fatalf("Hex String = %q, want 0123", got)
	}
}

func BenchmarkFilteredLogFmtHex(b *testing.B) {
	SetDebugLevel(DebugInfo)
	hash := bytes.Repeat([]byte{0xab}, 32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Log(DebugVerbose, "probe", "hash", fmt.Sprintf("%x", hash[:8]))
	}
}

func BenchmarkFilteredLogLazyHex(b *testing.B) {
	SetDebugLevel(DebugInfo)
	hash := bytes.Repeat([]byte{0xab}, 32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Log(DebugVerbose, "probe", "hash", Hex(hash[:8]))
	}
}
