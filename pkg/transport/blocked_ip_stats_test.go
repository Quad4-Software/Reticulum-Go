// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

type blockedStatsIface struct {
	mockInterface
	clients   int
	blocked   int
	blockedIP []string
}

func (b *blockedStatsIface) Clients() int { return b.clients }

func (b *blockedStatsIface) BlockedIPCount() int { return b.blocked }

func (b *blockedStatsIface) BlockedIPs() []string {
	return append([]string(nil), b.blockedIP...)
}

func TestGetInterfaceStatsRPCIncludesBlockedIPs(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	iface := &blockedStatsIface{
		clients:   3,
		blocked:   1,
		blockedIP: []string{"198.51.100.9"},
	}
	iface.Name = "BackboneInterface[blocked-stats]"
	iface.Enabled = true
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	stats := tr.GetInterfaceStatsRPC()
	var found *InterfaceStat
	for i := range stats.Interfaces {
		if stats.Interfaces[i].Name == iface.GetName() {
			found = &stats.Interfaces[i]
			break
		}
	}
	if found == nil {
		t.Fatal("interface missing from stats")
	}
	if found.Clients == nil || *found.Clients != 3 {
		t.Fatalf("Clients=%v want 3", found.Clients)
	}
	if found.BlockedIPs == nil || *found.BlockedIPs != 1 {
		t.Fatalf("BlockedIPs=%v want 1", found.BlockedIPs)
	}
	if len(found.BlockedIPList) != 1 || found.BlockedIPList[0] != "198.51.100.9" {
		t.Fatalf("BlockedIPList=%v", found.BlockedIPList)
	}
}
