// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"sync"
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestRaceBlockedIPStatsRPC(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	iface := &blockedStatsIface{
		clients:   1,
		blocked:   2,
		blockedIP: []string{"198.51.100.1", "198.51.100.2"},
	}
	iface.Name = "BackboneInterface[race-stats]"
	iface.Enabled = true
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				stats := tr.GetInterfaceStatsRPC()
				if len(stats.Interfaces) == 0 {
					t.Error("empty interfaces")
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestAdversarialBlockedIPStatsEmptyList(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })

	iface := &blockedStatsIface{clients: 0, blocked: 0, blockedIP: nil}
	iface.Name = "BackboneInterface[empty-block]"
	iface.Enabled = true
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatal(err)
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
		t.Fatal("missing iface")
	}
	if found.BlockedIPs == nil || *found.BlockedIPs != 0 {
		t.Fatalf("BlockedIPs=%v want 0", found.BlockedIPs)
	}
	if len(found.BlockedIPList) != 0 {
		t.Fatalf("BlockedIPList=%v want empty", found.BlockedIPList)
	}
}
