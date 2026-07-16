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
	for i := range 40 {
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

func TestAnalyzeSlowIntegrityAndAuthFindings(t *testing.T) {
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{
			{
				Name:               "Radio0",
				Status:             true,
				Bitrate:            50_000,
				IFACFail:           40,
				HMACFail:           10,
				RxOK:               50,
				IntegrityFailRate:  0.50,
				IntegritySamples60: 100,
				AnnounceSigFail:    12,
				ProofFail:          4,
				StaleCloses:        5,
				KeepaliveTimeout:   2,
			},
		},
	}
	rep := AnalyzeSlow(stats, nil, nil, "local", SlowAnalyzeOptions{
		MinIntegritySamples: 20,
		IntegrityWarnRate:   0.05,
		AuthFailWarn:        5,
		StaleWarn:           3,
	})
	kinds := map[string]bool{}
	for _, f := range rep.Findings {
		kinds[f.Kind] = true
	}
	for _, want := range []string{"integrity_burst", "auth_pressure", "link_degraded"} {
		if !kinds[want] {
			t.Fatalf("missing finding kind %q in %#v", want, rep.Findings)
		}
	}
	if rep.Interfaces[0].Score < 40 {
		t.Fatalf("Radio0 score=%.1f want elevated", rep.Interfaces[0].Score)
	}
}

func TestAnalyzeSlowQuietLowBitrateNotCriticalOnRTTAlone(t *testing.T) {
	rtt := 800.0
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{
			{
				Name:    "LoRa0",
				Status:  true,
				Bitrate: 1200,
				RTTMs:   &rtt,
			},
		},
	}
	rep := AnalyzeSlow(stats, nil, nil, "local", SlowAnalyzeOptions{})
	for _, f := range rep.Findings {
		if f.Kind == "integrity_burst" || f.Kind == "auth_pressure" || f.Kind == "link_degraded" {
			t.Fatalf("unexpected health finding on quiet radio: %#v", f)
		}
	}
}

func TestHealthFindingsForRowIngress(t *testing.T) {
	opts := SlowAnalyzeOptions{}
	opts.normalize()
	out := healthFindingsForRow(SlowIfaceRow{
		Name:          "tcp0",
		HeldAnnounces: 3,
		BurstActive:   true,
	}, opts)
	found := false
	for _, f := range out {
		if f.Kind == "ingress_pressure" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want ingress_pressure, got %#v", out)
	}
}
