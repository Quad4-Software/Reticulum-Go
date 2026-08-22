// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
)

type autoconnectLimitOracle struct {
	Count    int
	Backbone int
	TCP      int
}

func TestOracleAutoconnectRespectsMaxConcurrent(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 2
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	peers := []discovery.Info{
		{Type: "BackboneInterface", Name: "b1", ReachableOn: "192.0.2.10", Port: 1, HasPort: true, Transport: true},
		{Type: "BackboneInterface", Name: "b2", ReachableOn: "192.0.2.11", Port: 2, HasPort: true, Transport: true},
		{Type: "TCPServerInterface", Name: "t3", ReachableOn: "192.0.2.12", Port: 3, HasPort: true, Transport: true},
	}
	for _, p := range peers {
		n.autoconnect(&discovery.ReceivedAnnounceInfo{Info: p, RemoteIdentity: bytes.Repeat([]byte{0x01}, 16)})
	}
	oracle := autoconnectLimitOracle{Count: n.autoconnectCount()}
	n.acMu.Lock()
	for _, e := range n.acEntries {
		switch e.iface.(type) {
		case *interfaces.BackboneClientInterface:
			oracle.Backbone++
		case *interfaces.TCPClientInterface:
			oracle.TCP++
		}
	}
	n.acMu.Unlock()
	if oracle.Count != 2 {
		t.Fatalf("count=%d want 2", oracle.Count)
	}
	if oracle.Backbone != 2 || oracle.TCP != 0 {
		t.Fatalf("mix backbone=%d tcp=%d want 2/0", oracle.Backbone, oracle.TCP)
	}
}

func TestOracleAutoconnectExistsBlocksDuplicateHash(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 4
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type: "TCPServerInterface", ReachableOn: "192.0.2.20", Port: 5000, HasPort: true,
		},
	}
	n.autoconnect(info)
	before := n.autoconnectCount()
	n.autoconnect(info)
	if n.autoconnectCount() != before {
		t.Fatal("duplicate endpoint should not create second autoconnect")
	}
}

func TestOracleAutoconnectPeerConfigDefaults(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.EnableTransport = true
	cfg.AutoconnectInterfaceGravitySet = true
	cfg.AutoconnectInterfaceGravity = 42
	cfg.AutoconnectAnnouncesToInternalSet = true
	cfg.AutoconnectAnnouncesToInternal = true
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	peerCfg := n.autoconnectPeerConfig()
	if peerCfg.Bitrate != 5_000_000 || peerCfg.Gravity != 42 || !peerCfg.AnnouncesToInternal {
		t.Fatalf("peer cfg=%+v", peerCfg)
	}
	if peerCfg.Mode != "gateway" {
		t.Fatalf("mode=%q want gateway", peerCfg.Mode)
	}
}

func TestOracleAutoconnectMonitorClearsDownSince(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	tc, err := interfaces.NewTCPClientInterfaceWithRetries("offline", "127.0.0.1", 9, false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	entry := &autoconnectEntry{iface: tc, hash: []byte{0x01}, downSince: time.Now()}
	n.acMu.Lock()
	n.acEntries = []*autoconnectEntry{entry}
	n.acMu.Unlock()

	tc.Online = true
	n.autoconnectMonitorTick()
	if !entry.downSince.IsZero() {
		t.Fatal("online iface should clear downSince")
	}
}

func TestOracleAutoconnectMonitorDetachesStaleOffline(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	tc, err := interfaces.NewTCPClientInterfaceWithRetries("stale", "127.0.0.1", 9, false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	entry := &autoconnectEntry{
		iface:     tc,
		hash:      []byte{0x02},
		downSince: time.Now().Add(-autoconnectDetachAfter - time.Second),
	}
	n.acMu.Lock()
	n.acEntries = []*autoconnectEntry{entry}
	n.acMu.Unlock()

	n.autoconnectMonitorTick()
	if n.autoconnectCount() != 0 {
		t.Fatalf("stale entry count=%d want 0", n.autoconnectCount())
	}
}
