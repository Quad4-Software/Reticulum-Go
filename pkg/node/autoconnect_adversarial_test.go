// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
)

func TestAdversarialAutoconnectIgnoresUnknownType(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 3
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	before := n.autoconnectCount()
	n.autoconnect(&discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{Type: "AutoInterface", ReachableOn: "192.0.2.1", HasPort: true, Port: 1},
	})
	if n.autoconnectCount() != before {
		t.Fatal("AutoInterface must not autoconnect")
	}
}

func TestAdversarialAutoconnectSkipsEmptyReachableOn(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 2
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	n.autoconnect(&discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{Type: "BackboneInterface", Name: "empty", HasPort: true, Port: 1},
	})
	if n.autoconnectCount() != 0 {
		t.Fatalf("count=%d want 0 for empty host", n.autoconnectCount())
	}
}

func TestAdversarialAutoconnectDisabledWhenMaxZero(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 0
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	n.onInterfaceDiscovered(&discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type: "BackboneInterface", ReachableOn: "192.0.2.5", Port: 1, HasPort: true, Transport: true,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x01}, 16),
	})
	if n.autoconnectCount() != 0 {
		t.Fatal("autoconnect disabled when max is 0")
	}
}

func TestAdversarialAutoconnectNilNodeSafe(t *testing.T) {
	var n *Node
	n.autoconnect(nil)
	n.onInterfaceDiscovered(nil)
	if n.autoconnectCount() != 0 {
		t.Fatal("nil node must not panic or count")
	}
}
