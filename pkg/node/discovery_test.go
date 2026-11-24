// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
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
	cfg.StaticTransportIdentity = true
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
	defer func() { _ = n.Stop() }()
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

// TestDiscoverableDisabledWithoutStaticTransportIdentity covers upstream
// 606a1691: a discoverable interface on a non-transport instance that does
// not opt into static_transport_identity has discovery disabled instead of
// silently pinning the ephemeral identity, so restarts cannot register it
// as a new node on discovery services.
func TestDiscoverableDisabledWithoutStaticTransportIdentity(t *testing.T) {
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
	defer func() { _ = n.Stop() }()
	if cfg.StaticTransportIdentity {
		t.Fatal("discoverable interface must not gain static transport identity")
	}
	if cfg.Interfaces["pub"].Discoverable {
		t.Fatal("discoverable flag should be cleared on non-transport ephemeral instance")
	}
	if n.announcer != nil {
		t.Fatal("no announcer should start for the disabled discoverable interface")
	}

	// With static_transport_identity set the interface stays discoverable.
	fixed := common.DefaultConfig()
	fixed.EnableTransport = false
	fixed.StaticTransportIdentity = true
	fixed.Interfaces = map[string]*common.InterfaceConfig{
		"pub": {
			Type:         "TCPServerInterface",
			Enabled:      true,
			Discoverable: true,
			Port:         4244,
			ReachableOn:  "127.0.0.1",
		},
	}
	n2, err := New(fixed)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n2.Stop() }()
	if !fixed.Interfaces["pub"].Discoverable {
		t.Fatal("static transport identity must keep the interface discoverable")
	}
}
