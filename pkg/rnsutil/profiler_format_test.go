// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/profiler"
	"quad4/reticulum-go/pkg/transport"
)

func TestFormatProfilingResultsFromGoMap(t *testing.T) {
	profiler.Reset()
	defer profiler.Reset()
	profiler.Do("fmt.tag", func() {})
	res := profiler.Results()
	text := formatProfilingResults(res)
	if !strings.Contains(text, "fmt.tag") {
		t.Fatalf("format missing tag:\n%s", text)
	}
}

func TestWriteStatusHumanPPS(t *testing.T) {
	var buf bytes.Buffer
	stats := transport.InterfaceStatsResponse{
		RXB:   1000,
		TXB:   2000,
		RXS:   100,
		TXS:   200,
		RXPPS: 12,
		TXPPS: 34,
	}
	if err := WriteStatusHuman(&buf, stats, nil, nil, StatusOptions{TrafficTotals: true, ShowPPS: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "pps") {
		t.Fatalf("missing pps:\n%s", out)
	}
}

func TestNormalizeBlackholeRPCIgnoresEmpty(t *testing.T) {
	got := NormalizeBlackholeRPC([]map[string]any{
		{"identity": []byte{}, "until": 1.0},
		{"identity": mustHex(t, "00112233445566778899aabbccddeeff"), "until": 0.0, "reason": "x"},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func FuzzParseDestHash(f *testing.F) {
	f.Add("00112233445566778899aabbccddeeff")
	f.Add("")
	f.Add("zz")
	f.Add(strings.Repeat("ab", 16))
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseDestHash(s)
	})
}

func FuzzPrettyHex(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3})
	f.Add(bytes.Repeat([]byte{0xff}, 16))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = PrettyHex(b)
		_ = HexHash(b)
		_ = SizeString(float64(len(b)), "B")
	})
}

func FuzzFormatProfilingResults(f *testing.F) {
	f.Add("")
	f.Add("plain")
	f.Fuzz(func(t *testing.T, s string) {
		_ = formatProfilingResults(s)
		_ = formatProfilingResults([]byte(s))
		_ = formatProfilingResults(map[string]any{"name": s, "stats_all": map[string]any{"count": 1.0, "mean": 0.1}})
	})
}
