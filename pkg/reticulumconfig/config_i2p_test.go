// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package reticulumconfig

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestApplyInterfaceOptionI2P(t *testing.T) {
	cfg := &common.InterfaceConfig{}
	applyInterfaceOption(cfg, "peers", "a.b32.i2p, b.b32.i2p")
	applyInterfaceOption(cfg, "connectable", "true")
	applyInterfaceOption(cfg, "sam_address", "127.0.0.1:7656")
	if len(cfg.I2PPeers) != 2 || cfg.I2PPeers[0] != "a.b32.i2p" {
		t.Fatalf("peers: %#v", cfg.I2PPeers)
	}
	if !cfg.I2PConnectable {
		t.Fatal("expected connectable")
	}
	if cfg.I2PSAMAddress != "127.0.0.1:7656" {
		t.Fatalf("sam_address: %q", cfg.I2PSAMAddress)
	}
}
