// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
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

// TestDiscoverableEnablesStaticTransportIdentity covers the RNS 1.5.6 rule:
// a discoverable interface on a non-transport instance flips
// static_transport_identity on at config-apply time.
func TestDiscoverableEnablesStaticTransportIdentity(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.EnableTransport = false
	cfg.Interfaces = map[string]*common.InterfaceConfig{
		"pub": {
			Type:         "TCPServerInterface",
			Enabled:      true,
			Discoverable: true,
			Port:         4243,
			ReachableOn:  "127.0.0.1",
		},
	}
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	if !cfg.StaticTransportIdentity {
		t.Fatal("discoverable interface should force static transport identity")
	}

	// Without a discoverable interface the flag stays off.
	plain := common.DefaultConfig()
	plain.EnableTransport = false
	n2, err := New(plain)
	if err != nil {
		t.Fatal(err)
	}
	defer n2.Stop()
	if plain.StaticTransportIdentity {
		t.Fatal("non-discoverable config must not gain static transport identity")
	}
}
