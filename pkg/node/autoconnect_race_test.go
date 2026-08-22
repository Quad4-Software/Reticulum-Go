// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/discovery"
)

func TestRaceAutoconnectConcurrentSameEndpoint(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 4
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{
			Type:        "TCPServerInterface",
			Name:        "race-tcp",
			ReachableOn: "192.0.2.88",
			Port:        8888,
			HasPort:     true,
			Transport:   true,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x33}, 16),
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 30 {
				n.autoconnect(info)
			}
		})
	}
	wg.Wait()
	if n.autoconnectCount() != 1 {
		t.Fatalf("autoconnect count=%d want 1", n.autoconnectCount())
	}
}

func TestRaceAutoconnectMonitorAndConnect(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.AutoconnectDiscoveredInterfaces = 8
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	var seq atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				id := seq.Add(1)
				n.autoconnect(&discovery.ReceivedAnnounceInfo{
					Info: discovery.Info{
						Type:        "BackboneInterface",
						Name:        "race",
						ReachableOn: "192.0.2.100",
						Port:        int64(1000 + id),
						HasPort:     true,
						Transport:   true,
					},
					RemoteIdentity: bytes.Repeat([]byte{byte(id)}, 16),
				})
			}
		})
		wg.Go(func() {
			for range 40 {
				n.autoconnectMonitorTick()
			}
		})
	}
	wg.Wait()
	n.acMu.Lock()
	count := len(n.acEntries)
	n.acMu.Unlock()
	if count > cfg.AutoconnectDiscoveredInterfaces {
		t.Fatalf("entries=%d exceed max %d", count, cfg.AutoconnectDiscoveredInterfaces)
	}
}

func TestRaceAutoconnectExistsConcurrent(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()

	info := &discovery.ReceivedAnnounceInfo{
		Info: discovery.Info{Type: "BackboneInterface", ReachableOn: "192.0.2.77", Port: 4242, HasPort: true},
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 100 {
				_ = n.autoconnectExists(info)
				n.autoconnectCandidateIfaces()
			}
		})
	}
	wg.Wait()
}
