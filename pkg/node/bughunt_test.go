// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

// TestBughuntReloadEqualityCatchesHardwareFields ensures SIGHUP reload
// notices serial/SDR/modem73/discovery changes that previously compared equal.
func TestBughuntReloadEqualityCatchesHardwareFields(t *testing.T) {
	base := &common.InterfaceConfig{Type: "SerialInterface", Enabled: true, Device: "/dev/ttyUSB0"}
	cases := []struct {
		name string
		mut  func(*common.InterfaceConfig)
	}{
		{"Device", func(c *common.InterfaceConfig) { c.Device = "/dev/ttyUSB1" }},
		{"Speed", func(c *common.InterfaceConfig) { c.Speed = 115200 }},
		{"FrequencyHz", func(c *common.InterfaceConfig) { c.FrequencyHz = 433000000 }},
		{"SampleRate", func(c *common.InterfaceConfig) { c.SampleRate = 2000000 }},
		{"Modem", func(c *common.InterfaceConfig) { c.Modem = "burst" }},
		{"ControlHost", func(c *common.InterfaceConfig) { c.ControlHost = "127.0.0.1" }},
		{"ControlPort", func(c *common.InterfaceConfig) { c.ControlPort = 8001 }},
		{"Path", func(c *common.InterfaceConfig) { c.Path = "/rns2" }},
		{"Domain", func(c *common.InterfaceConfig) { c.Domain = "mesh.example" }},
		{"Discoverable", func(c *common.InterfaceConfig) { c.Discoverable = true }},
		{"DiscoveryName", func(c *common.InterfaceConfig) { c.DiscoveryName = "lab" }},
		{"ContextID", func(c *common.InterfaceConfig) { c.ContextID = 3 }},
	}
	for _, tc := range cases {
		clone := *base
		tc.mut(&clone)
		if interfaceConfigsEqualForReload(base, &clone) {
			t.Fatalf("%s change treated as equal (reload would no-op)", tc.name)
		}
	}
}
