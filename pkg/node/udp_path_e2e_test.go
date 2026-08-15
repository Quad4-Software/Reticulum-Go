// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"fmt"
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return port
}

// TestUDPPathE2E starts two nodes over loopback UDP, announces from A, and
// waits until B learns a path. This is a black-box node API end-to-end check.
func TestUDPPathE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping UDP path e2e in -short mode")
	}
	portA := freeUDPPort(t)
	portB := freeUDPPort(t)

	cfgA := common.DefaultConfig()
	cfgA.ShareInstance = false
	cfgA.EnableControlAPI = false
	cfgA.Interfaces = map[string]*common.InterfaceConfig{
		"udp": {
			Name:       "udp",
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("127.0.0.1:%d", portA),
			TargetHost: "127.0.0.1",
			TargetPort: portB,
		},
	}
	cfgB := common.DefaultConfig()
	cfgB.ShareInstance = false
	cfgB.EnableControlAPI = false
	cfgB.Interfaces = map[string]*common.InterfaceConfig{
		"udp": {
			Name:       "udp",
			Type:       "UDPInterface",
			Enabled:    true,
			Address:    fmt.Sprintf("127.0.0.1:%d", portB),
			TargetHost: "127.0.0.1",
			TargetPort: portA,
		},
	}

	nodeA, err := New(cfgA)
	if err != nil {
		t.Fatalf("node A: %v", err)
	}
	nodeB, err := New(cfgB)
	if err != nil {
		t.Fatalf("node B: %v", err)
	}
	defer func() { _ = nodeA.Stop() }()
	defer func() { _ = nodeB.Stop() }()

	if err := nodeA.Start(); err != nil {
		t.Fatalf("start A: %v", err)
	}
	if err := nodeB.Start(); err != nil {
		t.Fatalf("start B: %v", err)
	}

	idA, err := identity.New()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, "e2eapp", nodeA.Transport(), "path")
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	if err := destA.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	hash := destA.GetHash()
	if len(hash) == 0 {
		t.Fatal("empty destination hash")
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if nodeB.Transport().HasPath(hash) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("node B did not learn path to %x", hash[:8])
}
