// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/transport"
)

func TestAnalyzeSlowRanksSaturatedInterface(t *testing.T) {
	stats := transport.InterfaceStatsResponse{
		TransportID:     []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		TransportUptime: 120,
		RXS:             180_000,
		TXS:             10_000,
		Interfaces: []transport.InterfaceStat{
			{
				Name:    "FastUplink",
				Type:    "TCPClientInterface",
				Status:  true,
				Bitrate: 10_000_000,
				RXS:     50_000,
				TXS:     40_000,
			},
			{
				Name:          "SlowPeer",
				Type:          "TCPClientInterface",
				Status:        true,
				Bitrate:       200_000,
				RXS:           180_000,
				TXS:           20_000,
				BurstActive:   true,
				HeldAnnounces: 8,
			},
		},
	}
	paths := make([]transport.PathTableEntry, 0, 40)
	for i := 0; i < 40; i++ {
		h := make([]byte, 16)
		h[0] = byte(i)
		paths = append(paths, transport.PathTableEntry{
			Hash:      h,
			Hops:      8,
			Interface: "SlowPeer",
			Via:       []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		})
	}

	rep := AnalyzeSlow(stats, paths, nil, "127.0.0.1:37429", SlowAnalyzeOptions{})
	if len(rep.Interfaces) < 1 {
		t.Fatal("expected ranked interfaces")
	}
	if rep.Interfaces[0].Name != "SlowPeer" {
		t.Fatalf("top interface = %q, want SlowPeer", rep.Interfaces[0].Name)
	}
	if rep.Interfaces[0].Score < 50 {
		t.Fatalf("SlowPeer score = %.1f, want high", rep.Interfaces[0].Score)
	}
	if rep.PathStats.HighHop != 40 {
		t.Fatalf("high hop paths = %d, want 40", rep.PathStats.HighHop)
	}
	if len(rep.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
	var buf strings.Builder
	if err := WriteSlowHuman(&buf, rep); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "SlowPeer") || !strings.Contains(out, "RECOMMENDATIONS") {
		t.Fatalf("human output missing expected sections:\n%s", out)
	}
}
