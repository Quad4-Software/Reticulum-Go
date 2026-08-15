// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestNewFromConfigI2PInterface(t *testing.T) {
	iface, err := NewFromConfigWithContext("i2p0", &common.InterfaceConfig{
		Type:           "I2PInterface",
		Enabled:        true,
		I2PConnectable: true,
		I2PPeers:       []string{"peer1.b32.i2p"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, ok := iface.(*I2PInterface)
	if !ok {
		t.Fatalf("expected *I2PInterface, got %T", iface)
	}
	defer parent.Stop()
	if parent.bindPort == 0 {
		t.Fatal("expected bind port")
	}
	if len(parent.spawned) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(parent.spawned))
	}
}
