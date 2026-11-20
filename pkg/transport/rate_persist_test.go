// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"
)

func TestRateTablePersistRoundTrip(t *testing.T) {
	var k1, k2 [16]byte
	k1[0] = 0x11
	k2[0] = 0x22
	entries := map[[16]byte]rateTableEntry{
		k1: {last: 100.5, blocked: 200.0, viol: 3, timestamps: []float64{1, 2, 3}},
		k2: {last: 300.0, blocked: 0, viol: 0, timestamps: []float64{4, 5}},
	}
	blob := marshalRateTable(entries)
	got, ok := unmarshalRateTable(blob)
	if !ok {
		t.Fatal("unmarshalRateTable rejected own output")
	}
	if len(got) != 2 {
		t.Fatalf("entries=%d want 2", len(got))
	}
	e := got[k1]
	if e.last != 100.5 || e.blocked != 200.0 || e.viol != 3 {
		t.Fatalf("entry mismatch: %+v", e)
	}
	if len(e.timestamps) != 3 || e.timestamps[0] != 1 || e.timestamps[2] != 3 {
		t.Fatalf("timestamps=%v", e.timestamps)
	}
}

func TestRateTablePersistRejectsBad(t *testing.T) {
	if _, ok := unmarshalRateTable([]byte("short")); ok {
		t.Fatal("accepted truncated blob")
	}
	bad := marshalRateTable(map[[16]byte]rateTableEntry{})
	bad[6] = 0x99
	if _, ok := unmarshalRateTable(bad); ok {
		t.Fatal("accepted wrong version")
	}
}

// TestRateTablePersistRingFull exercises ring-ordered decode (ts_len == max).
func TestRateTablePersistRingFull(t *testing.T) {
	var k [16]byte
	k[0] = 0xaa
	ts := make([]float64, rateTableMaxTS)
	for i := range ts {
		ts[i] = float64(i + 1)
	}
	entries := map[[16]byte]rateTableEntry{
		k: {last: 16, timestamps: ts},
	}
	blob := marshalRateTable(entries)
	got, ok := unmarshalRateTable(blob)
	if !ok {
		t.Fatal("unmarshalRateTable rejected full-ring entry")
	}
	e := got[k]
	if len(e.timestamps) != rateTableMaxTS || e.timestamps[rateTableMaxTS-1] != float64(rateTableMaxTS) {
		t.Fatalf("timestamps=%v", e.timestamps)
	}
}

// TestRateTableLenCheckOverflow guards the 32-bit path: a crafted count can
// wrap a 32-bit int product so the old expression len(blob) == 16+count*entry
// passed, then make([][16]byte, count) allocated gigabytes. The check must be
// computed in 64-bit.
func TestRateTableLenCheckOverflow(t *testing.T) {
	// count*entrySize wraps past 2^32: 93392930*46 = 4296074780.
	// On a 32-bit int this wraps to 1107484, +16 = 1107500, which would
	// accept a 1107500-byte blob and then allocate ~1.5G entries.
	if rateTableCountFits(1107500, 93392930, 16, 1) {
		t.Fatal("crafted count that overflows a 32-bit product was accepted")
	}
	if !rateTableCountFits(16+3*46, 3, 16, 1) {
		t.Fatal("valid small table rejected")
	}
	if rateTableCountFits(16+3*46+1, 3, 16, 1) {
		t.Fatal("off-by-one length accepted")
	}
}
