// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package node

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestAutoconnectI2PCreatesPeer(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 2
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	dest := "peer.b32.i2p"
	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "I2PInterface",
			Name:        "i2p peer",
			ReachableOn: dest,
			Transport:   true,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x22}, 16),
	}
	n.autoconnect(info)
	if n.autoconnectCount() != 1 {
		t.Fatalf("autoconnect count %d want 1", n.autoconnectCount())
	}
	n.acMu.Lock()
	entry := n.acEntries[0]
	n.acMu.Unlock()
	peer, ok := entry.iface.(*interfaces.I2PInterfacePeer)
	if !ok {
		t.Fatalf("expected I2PInterfacePeer, got %T", entry.iface)
	}
	if peer.TargetDest() != dest {
		t.Fatalf("dest %q", peer.TargetDest())
	}
}
