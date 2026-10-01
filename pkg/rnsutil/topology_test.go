// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func TestWriteTopologyHumanGrouping(t *testing.T) {
	now := time.Now()
	stats := transport.InterfaceStatsResponse{
		Interfaces: []transport.InterfaceStat{
			{Name: "udp0", Status: true},
			{Name: "tcp0", Status: false},
		},
	}
	mk := func(h byte, iface string, hops uint8, ts float64) transport.PathTableEntry {
		return transport.PathTableEntry{
			Hash:      []byte{h, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
			Interface: iface,
			Hops:      hops,
			Timestamp: ts,
			Via:       []byte{9, 9},
			Expires:   float64(now.Add(time.Hour).Unix()),
		}
	}
	paths := []transport.PathTableEntry{
		mk(0xA, "udp0", 3, float64(now.Add(-time.Minute).Unix())),
		mk(0xB, "udp0", 1, float64(now.Unix())),
		mk(0xC, "tcp0", 2, float64(now.Unix())),
	}
	var buf bytes.Buffer
	if err := WriteTopologyHuman(&buf, stats, paths, now); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Topology: 3 paths via 2 interfaces") {
		t.Fatalf("bad summary: %s", out)
	}
	// udp0 sorts first (2 paths) and shows both rows sorted by hops.
	udpIdx := strings.Index(out, "udp0 (online) - 2 paths, hops 1-3")
	if udpIdx < 0 {
		t.Fatalf("missing udp0 header: %s", out)
	}
	if !strings.Contains(out, "tcp0 (offline) - 1 path") {
		t.Fatalf("missing tcp0 header: %s", out)
	}
	if !strings.Contains(out, "hops=1") || !strings.Contains(out, "via") {
		t.Fatalf("missing row detail: %s", out)
	}
	if !strings.Contains(out, "exp in") {
		t.Fatalf("missing expiry: %s", out)
	}
}

func TestWriteTopologyHumanEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTopologyHuman(&buf, transport.InterfaceStatsResponse{}, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Topology: 0 paths via 0 interfaces") {
		t.Fatalf("bad empty output: %q", buf.String())
	}
}

func TestWriteTopologyHumanExpired(t *testing.T) {
	now := time.Now()
	paths := []transport.PathTableEntry{{
		Hash:      []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		Interface: "udp0",
		Hops:      1,
		Timestamp: float64(now.Add(-time.Hour).Unix()),
		Expires:   float64(now.Add(-time.Minute).Unix()),
	}}
	var buf bytes.Buffer
	if err := WriteTopologyHuman(&buf, transport.InterfaceStatsResponse{}, paths, now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(expired)") {
		t.Fatalf("missing expired marker: %s", buf.String())
	}
}
