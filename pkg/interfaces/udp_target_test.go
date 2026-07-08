// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestUDPTargetFromConfigUsesTargetAddress(t *testing.T) {
	cfg := &common.InterfaceConfig{
		Type:          "UDPInterface",
		Address:       "127.0.0.1:0",
		TargetAddress: "127.0.0.1:4242",
		TargetHost:    "127.0.0.1:1111",
		Enabled:       false,
	}
	iface, err := NewFromConfig("udp-test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ui := iface.(*UDPInterface)
	if ui.targetAddr == nil || ui.targetAddr.String() != "127.0.0.1:4242" {
		t.Fatalf("target = %v, want 127.0.0.1:4242", ui.targetAddr)
	}
}

func TestUDPNoAutoPeerBinding(t *testing.T) {
	ui, err := NewUDPInterface("u", "127.0.0.1:0", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if ui.targetAddr != nil {
		t.Fatalf("expected nil target without explicit peer, got %v", ui.targetAddr)
	}
}
