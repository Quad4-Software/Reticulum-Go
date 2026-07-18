// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"math"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestParseNodeStatusAppDataRejectsOutOfRangeMaxSize(t *testing.T) {
	cases := []struct {
		name string
		in   []any
		ok   bool
	}{
		{"valid", []any{true, int64(100), int64(500)}, true},
		{"min_int16", []any{false, int64(1), int64(math.MinInt16)}, true},
		{"max_int16", []any{true, int64(1), int64(math.MaxInt16)}, true},
		{"overflow", []any{true, int64(1), int64(math.MaxInt16) + 1}, false},
		{"underflow", []any{true, int64(1), int64(math.MinInt16) - 1}, false},
		{"short", []any{true, int64(1)}, false},
		{"bad_enabled", []any{"yes", int64(1), int64(1)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, maxKB, ok := parseNodeStatusAppData(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v (maxKB=%d)", ok, tc.ok, maxKB)
			}
			if ok && tc.name == "valid" && maxKB != 500 {
				t.Fatalf("maxKB=%d want 500", maxKB)
			}
		})
	}
}

func TestCreateNodeAppDataRoundTrip(t *testing.T) {
	r := &Reticulum{nodeEnabled: true, maxTransferSize: 750}
	raw := r.createNodeAppData()
	var decoded any
	if err := msgpack.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	enabled, _, maxKB, ok := parseNodeStatusAppData(decoded)
	if !ok {
		t.Fatal("parse failed on self-encoded appData")
	}
	if !enabled || maxKB != 750 {
		t.Fatalf("enabled=%v maxKB=%d", enabled, maxKB)
	}
}

// FuzzParseNodeStatusAppDataExploratory requires successful parses to keep maxSize
// inside int16 without silent truncation.
func FuzzParseNodeStatusAppDataExploratory(f *testing.F) {
	if raw, err := msgpack.Marshal([]any{true, int64(42), int64(100)}); err == nil {
		f.Add(raw)
	}
	if raw, err := msgpack.Marshal([]any{false, int64(1), int64(math.MaxInt16) + 9}); err == nil {
		f.Add(raw)
	}
	f.Add([]byte{})
	f.Add([]byte{0x93, 0xc3, 0x01, 0x02})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<12 {
			t.Skip()
		}
		var decoded any
		if err := msgpack.Unmarshal(raw, &decoded); err != nil {
			return
		}
		_, _, maxKB, ok := parseNodeStatusAppData(decoded)
		if !ok {
			return
		}
		arr, isArr := decoded.([]any)
		if !isArr || len(arr) < 3 {
			t.Fatal("ok with non-array shape")
		}
		v, okInt := msgpackInt64(arr[2])
		if !okInt {
			t.Fatal("ok without int max size")
		}
		if v < math.MinInt16 || v > math.MaxInt16 {
			t.Fatalf("accepted out-of-range maxSize %d as %d", v, maxKB)
		}
		if int64(maxKB) != v {
			t.Fatalf("truncated maxSize: got %d from %d", maxKB, v)
		}
	})
}
