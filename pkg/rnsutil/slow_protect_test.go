// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/protect"
	"quad4/reticulum-go/pkg/transport"
)

func TestAnalyzeSlowProtectFinding(t *testing.T) {
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{{Name: "lo", Status: true, Bitrate: 1_000_000}},
		Protect: protect.Snapshot{
			Mode:        "auto",
			Phase:       "armed",
			Enforcement: "prevent",
			TripCounts: protect.TripCountsSnapshot{
				PPS: 12,
			},
		},
	}
	rep := AnalyzeSlow(stats, nil, nil, "", SlowAnalyzeOptions{})
	var found bool
	for _, f := range rep.Findings {
		if f.Kind == "dos_armed_trips" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("findings=%v", rep.Findings)
	}
}

func TestAnalyzeSlowProtectOffNoFinding(t *testing.T) {
	stats := transport.InterfaceStatsResponse{
		Protect: protect.Snapshot{Mode: "off"},
	}
	rep := AnalyzeSlow(stats, nil, nil, "", SlowAnalyzeOptions{})
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.Kind, "dos_") {
			t.Fatalf("unexpected %s", f.Kind)
		}
	}
}
