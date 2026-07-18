// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestInterfaceConfigsEqualForReloadOracle(t *testing.T) {
	if !interfaceConfigsEqualForReload(nil, nil) {
		t.Fatal("nil/nil must be equal")
	}
	a := &common.InterfaceConfig{Type: "UDPInterface", Enabled: true, Port: 4242}
	if interfaceConfigsEqualForReload(a, nil) || interfaceConfigsEqualForReload(nil, a) {
		t.Fatal("nil vs non-nil must differ")
	}
	b := &common.InterfaceConfig{Type: "UDPInterface", Enabled: true, Port: 4242}
	if !interfaceConfigsEqualForReload(a, b) {
		t.Fatal("identical configs must be equal")
	}
	if !interfaceConfigsEqualForReload(a, a) {
		t.Fatal("reflexive equality failed")
	}

	mutators := []func(*common.InterfaceConfig){
		func(c *common.InterfaceConfig) { c.Type = "TCPClientInterface" },
		func(c *common.InterfaceConfig) { c.Enabled = false },
		func(c *common.InterfaceConfig) { c.Port = 1 },
		func(c *common.InterfaceConfig) { c.Address = "10.0.0.1" },
		func(c *common.InterfaceConfig) { c.TargetHost = "peer.example" },
		func(c *common.InterfaceConfig) { c.MTU = 500 },
		func(c *common.InterfaceConfig) { c.Bitrate = 1200 },
		func(c *common.InterfaceConfig) { c.Mode = "gateway" },
		func(c *common.InterfaceConfig) { c.IFACSize = 16 },
		func(c *common.InterfaceConfig) { c.NetworkName = "lab" },
		func(c *common.InterfaceConfig) { c.Passphrase = "x" },
		func(c *common.InterfaceConfig) { c.Devices = []string{"eth0"} },
		func(c *common.InterfaceConfig) { c.AnnounceCap = 0.5 },
		func(c *common.InterfaceConfig) { c.Outgoing = false; c.OutgoingSet = true },
		func(c *common.InterfaceConfig) { c.Device = "/dev/ttyUSB9" },
		func(c *common.InterfaceConfig) { c.FrequencyHz = 915000000 },
		func(c *common.InterfaceConfig) { c.ControlHost = "10.0.0.2" },
		func(c *common.InterfaceConfig) { c.Discoverable = true },
	}
	for i, mutate := range mutators {
		clone := *a
		mutate(&clone)
		if interfaceConfigsEqualForReload(a, &clone) {
			t.Fatalf("mutator %d left configs equal", i)
		}
		if interfaceConfigsEqualForReload(&clone, a) != interfaceConfigsEqualForReload(a, &clone) {
			t.Fatalf("mutator %d broke symmetry", i)
		}
	}
}

func TestFloatEqualOracle(t *testing.T) {
	if !floatEqual(1.0, 1.0) {
		t.Fatal("identical floats")
	}
	if !floatEqual(1.0, 1.0+1e-10) {
		t.Fatal("within eps")
	}
	if floatEqual(1.0, 1.0+1e-6) {
		t.Fatal("outside eps")
	}
}

func TestSliceEqualOracle(t *testing.T) {
	if !sliceEqual(nil, nil) {
		t.Fatal("nil slices")
	}
	if sliceEqual([]string{"a"}, nil) {
		t.Fatal("nil vs non-nil")
	}
	if !sliceEqual([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("equal slices")
	}
	if sliceEqual([]string{"a"}, []string{"b"}) {
		t.Fatal("different slices")
	}
}
