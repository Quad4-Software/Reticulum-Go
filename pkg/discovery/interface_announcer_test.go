// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestHasDiscoverableInterfaces(t *testing.T) {
	cfg := &common.ReticulumConfig{
		Interfaces: map[string]*common.InterfaceConfig{
			"gw": {
				Type:         "TCPServerInterface",
				Enabled:      true,
				Discoverable: true,
				Port:         4242,
				ReachableOn:  "127.0.0.1",
			},
		},
	}
	if !HasDiscoverableInterfaces(cfg) {
		t.Fatal("expected discoverable interface")
	}
	cfg.Interfaces["gw"].Discoverable = false
	if HasDiscoverableInterfaces(cfg) {
		t.Fatal("expected no discoverable interface")
	}
}

func TestInterfaceAnnouncerBuildAnnounceData(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetIdentity(id)
	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		Interfaces: map[string]*common.InterfaceConfig{
			"pub": {
				Type:                         "TCPServerInterface",
				Enabled:                      true,
				Discoverable:                 true,
				Port:                         4242,
				ReachableOn:                  "example.org",
				DiscoveryName:                "test-gw",
				DiscoveryAnnounceIntervalSec: 300,
			},
		},
	}
	ann, err := NewInterfaceAnnouncer(tr, cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	appData, err := ann.buildAnnounceData(cfg.Interfaces["pub"])
	if err != nil {
		t.Fatalf("buildAnnounceData: %v", err)
	}
	if len(appData) <= 1+StampSize {
		t.Fatalf("app_data too short: %d", len(appData))
	}
	info, err := ValidateAndDecode(appData, DefaultStampValue, WorkblockExpandRounds)
	if err != nil {
		t.Fatalf("ValidateAndDecode: %v", err)
	}
	if info.Info.Type != "TCPServerInterface" {
		t.Fatalf("type=%s", info.Info.Type)
	}
	if info.Info.ReachableOn != "example.org" {
		t.Fatalf("reachable_on=%s", info.Info.ReachableOn)
	}
	if info.Info.Port != 4242 {
		t.Fatalf("port=%d", info.Info.Port)
	}
}

func TestInterfaceAnnouncerAnnounceDue(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetIdentity(id)
	cfg := &common.ReticulumConfig{
		EnableTransport: true,
		Interfaces: map[string]*common.InterfaceConfig{
			"pub": {
				Type:                         "TCPServerInterface",
				Enabled:                      true,
				Discoverable:                 true,
				Port:                         4242,
				ReachableOn:                  "127.0.0.1",
				DiscoveryAnnounceIntervalSec: 300,
			},
		},
	}
	ann, err := NewInterfaceAnnouncer(tr, cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	ann.now = func() time.Time { return now }
	ann.announceDue()
	ann.mu.Lock()
	_, ok := ann.lastAnnounce["pub"]
	ann.mu.Unlock()
	if !ok {
		t.Fatal("expected lastAnnounce for pub")
	}
	ann.announceDue()
	ann.mu.Lock()
	last := ann.lastAnnounce["pub"]
	ann.mu.Unlock()
	if !last.Equal(now) {
		t.Fatalf("interval should suppress second announce, last=%v", last)
	}
}
