// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestSelectInterfacesAll(t *testing.T) {
	cfg := &common.ReticulumConfig{
		Interfaces: map[string]*common.InterfaceConfig{
			"tcp": {Enabled: true, Type: "TCPServerInterface"},
			"udp": {Enabled: false, Type: "UDPInterface"},
		},
	}
	names, err := SelectInterfaces(cfg, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "tcp" {
		t.Fatalf("names=%v", names)
	}
	if !cfg.Interfaces["tcp"].Enabled || cfg.Interfaces["udp"].Enabled {
		t.Fatal("all should not flip Enabled flags")
	}
}

func TestSelectInterfacesSpecific(t *testing.T) {
	cfg := &common.ReticulumConfig{
		Interfaces: map[string]*common.InterfaceConfig{
			"tcp": {Enabled: true, Type: "TCPServerInterface"},
			"udp": {Enabled: true, Type: "UDPInterface"},
			"ws":  {Enabled: true, Type: "WebsocketClientInterface"},
		},
	}
	names, err := SelectInterfaces(cfg, "UDP, tcp")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("names=%v", names)
	}
	if !cfg.Interfaces["tcp"].Enabled || !cfg.Interfaces["udp"].Enabled {
		t.Fatal("expected tcp and udp enabled")
	}
	if cfg.Interfaces["ws"].Enabled {
		t.Fatal("ws should be disabled")
	}
}

func TestSelectInterfacesUnknown(t *testing.T) {
	cfg := &common.ReticulumConfig{
		Interfaces: map[string]*common.InterfaceConfig{
			"tcp": {Enabled: true, Type: "TCPServerInterface"},
		},
	}
	if _, err := SelectInterfaces(cfg, "missing"); err == nil {
		t.Fatal("expected error for unknown iface")
	}
}
