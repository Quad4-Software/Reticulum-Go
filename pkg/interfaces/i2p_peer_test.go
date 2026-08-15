// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import (
	"testing"

	"quad4/reticulum-go/pkg/common"
)

func TestI2PInterfacePeerParentCount(t *testing.T) {
	parent, err := NewI2PInterface("i2p0", &common.InterfaceConfig{
		Type:    "I2PInterface",
		Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()
	out := NewI2PInterfacePeer(parent, "i2p0 to peer.b32.i2p", "peer.b32.i2p", 0, parent.cfg)
	defer out.Stop()
	if out.parentCount {
		t.Fatal("outbound peer should not roll up stats to parent")
	}
	in := newI2PInterfacePeerAccepted(parent, "Connected peer on i2p0", nil)
	if !in.parentCount {
		t.Fatal("inbound peer should roll up stats to parent")
	}
}

func TestI2PInterfacePeerWantsTunnel(t *testing.T) {
	parent, err := NewI2PInterface("i2p0", &common.InterfaceConfig{
		Type:    "I2PInterface",
		Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()
	peer := NewI2PInterfacePeer(parent, "i2p0 to x.b32.i2p", "x.b32.i2p", 0, &common.InterfaceConfig{
		KISSFraming: false,
	})
	defer peer.Stop()
	if !peer.wantsTunnel {
		t.Fatal("HDLC peer should want tunnel synthesis")
	}
	kiss := NewI2PInterfacePeer(parent, "i2p0 to y.b32.i2p", "y.b32.i2p", 0, &common.InterfaceConfig{
		KISSFraming: true,
	})
	defer kiss.Stop()
	if kiss.wantsTunnel {
		t.Fatal("KISS peer should not want tunnel synthesis")
	}
}

func TestI2PInterfacePeerInterfaceHash(t *testing.T) {
	h := InterfaceHashFromName("i2p0 to peer.b32.i2p")
	if len(h) != 32 {
		t.Fatalf("hash len = %d, want 32", len(h))
	}
}

func TestI2PInterfaceSupportsDiscovery(t *testing.T) {
	parent, err := NewI2PInterface("i2p0", &common.InterfaceConfig{Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !parent.SupportsDiscovery() {
		t.Fatal("I2P parent should support discovery")
	}
}
