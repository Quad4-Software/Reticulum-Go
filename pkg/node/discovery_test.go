// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestStartInterfaceDiscovery(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.DiscoverInterfaces = true
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n.StartInterfaceDiscovery()
	if n.discovery == nil {
		t.Fatal("expected interface discovery listener")
	}
	n.StartInterfaceDiscovery()
	n.discovery.Stop()
}

func TestStartInterfaceDiscoveryDisabled(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.DiscoverInterfaces = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n.StartInterfaceDiscovery()
	if n.discovery != nil {
		t.Fatal("discovery should not start when disabled")
	}
}

func TestStartInterfaceDiscoveryFromDiscoverable(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.DiscoverInterfaces = false
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		"pub": {
			Type:         "TCPServerInterface",
			Enabled:      true,
			Discoverable: true,
			Port:         4242,
			ReachableOn:  "127.0.0.1",
		},
	}
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	// Transport identity is created on Start. Seed one for announcer.
	if err := n.Transport().InitializePathRequestHandler(); err != nil {
		t.Fatal(err)
	}
	n.StartInterfaceDiscovery()
	if n.discovery == nil {
		t.Fatal("expected discovery listener from discoverable interface")
	}
	if n.announcer == nil {
		t.Fatal("expected interface announcer")
	}
	n.announcer.Stop()
	n.discovery.Stop()
}
