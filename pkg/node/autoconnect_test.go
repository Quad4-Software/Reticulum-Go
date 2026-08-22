// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestStartInterfaceDiscoveryFromAutoconnectOnly(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.DiscoverInterfaces = false
	cfg.AutoconnectDiscoveredInterfaces = 2
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	n.StartInterfaceDiscovery()
	if n.discovery == nil {
		t.Fatal("expected discovery listener when autoconnect is enabled")
	}
}

func TestAutoconnectSkipsYgg(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 4
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	before := n.autoconnectCount()
	n.autoconnect(&discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{Type: "BackboneInterface", ReachableOn: "200::1", HasPort: true, Port: 4242},
	})
	if n.autoconnectCount() != before {
		t.Fatal("Yggdrasil autoconnect should be skipped")
	}
}

func TestAutoconnectTCPClientFromTCPServerAnnounce(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 2
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "TCPServerInterface",
			Name:        "tcp peer",
			ReachableOn: "192.0.2.50",
			Port:        4242,
			HasPort:     true,
			Transport:   true,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x11}, 16),
	}
	n.autoconnect(info)
	if n.autoconnectCount() != 1 {
		t.Fatalf("autoconnect count %d want 1", n.autoconnectCount())
	}
	n.acMu.Lock()
	entry := n.acEntries[0]
	n.acMu.Unlock()
	tc, ok := entry.iface.(*interfaces.TCPClientInterface)
	if !ok {
		t.Fatalf("expected TCPClientInterface, got %T", entry.iface)
	}
	if tc.TargetHost() != "192.0.2.50" || tc.TargetPort() != 4242 {
		t.Fatalf("target %s:%d", tc.TargetHost(), tc.TargetPort())
	}
}

func TestAutoconnectExistsDetectsTCPClient(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "TCPServerInterface",
			ReachableOn: "192.0.2.60",
			Port:        9000,
			HasPort:     true,
		},
	}
	tc, err := interfaces.NewTCPClientInterfaceWithRetries("manual", "192.0.2.60", 9000, false, false, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	n.reloadMu.Lock()
	n.interfaces = append(n.interfaces, tc)
	n.reloadMu.Unlock()
	if !n.autoconnectExists(info) {
		t.Fatal("expected existing tcp client to match")
	}
}

func TestOnInterfaceDiscoveredPersists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte("[reticulum]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := common.DefaultConfig()
	cfg.ConfigPath = cfgPath
	cfg.AutoconnectDiscoveredInterfaces = 1
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "BackboneInterface",
			Name:        "peer",
			ReachableOn: "192.0.2.9",
			Port:        7777,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0xab}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0xcd}, 16),
	}
	n.onInterfaceDiscovered(info)

	list, err := discovery.LoadPersistedInterfaces(n.discoveryStorageDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("persisted %d want 1", len(list))
	}
}

func TestStopAutoconnectMonitor(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 1
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	n.ensureAutoconnectMonitor()
	n.stopAutoconnectMonitor()
	n.acMu.Lock()
	running := n.acMonitorRunning
	n.acMu.Unlock()
	if running {
		t.Fatal("monitor should stop")
	}
}
