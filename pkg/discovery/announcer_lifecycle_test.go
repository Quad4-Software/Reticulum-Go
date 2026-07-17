// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

func TestInterfaceAnnouncerStartStop(t *testing.T) {
	tr := transport.NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	tr.SetIdentity(id)
	cfg := &common.ReticulumConfig{EnableTransport: true, Interfaces: map[string]*common.InterfaceConfig{}}
	ann, err := NewInterfaceAnnouncer(tr, cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	ann.jobInterval = 15 * time.Millisecond
	ann.Start()
	ann.Start() // second Start must be a no-op
	time.Sleep(40 * time.Millisecond)
	ann.Stop()
	ann.Stop() // second Stop must be safe
}

func TestNormalizeIfaceType(t *testing.T) {
	cases := map[string]string{
		"backbone":             "BackboneInterface",
		"BackboneInterface":    "BackboneInterface",
		"tcpserver":            "TCPServerInterface",
		"TCPServerInterface":   "TCPServerInterface",
		"tcpclient":            "TCPClientInterface",
		"i2p":                  "I2PInterface",
		"rnode":                "RNodeInterface",
		"weave":                "WeaveInterface",
		"kiss":                 "KISSInterface",
		"CustomThing":          "CustomThing",
		"  TCPServerInterface": "TCPServerInterface",
	}
	for in, want := range cases {
		if got := normalizeIfaceType(in); got != want {
			t.Errorf("normalizeIfaceType(%q) = %q, want %q", in, got, want)
		}
	}
	if !isDiscoverableType("tcpserver") {
		t.Fatal("tcpserver should be discoverable")
	}
	if isDiscoverableType("UDPInterface") {
		t.Fatal("UDPInterface should not be discoverable")
	}
}

func TestResolveReachableOn(t *testing.T) {
	got, err := resolveReachableOn("")
	if err != nil || got != "" {
		t.Fatalf("empty: got %q err %v", got, err)
	}
	got, err = resolveReachableOn("example.org")
	if err != nil || got != "example.org" {
		t.Fatalf("literal: got %q err %v", got, err)
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "reachable.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'pub.example.net'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = resolveReachableOn(script)
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	if got != "pub.example.net" {
		t.Fatalf("script output = %q", got)
	}

	emptyScript := filepath.Join(dir, "empty.sh")
	if err := os.WriteFile(emptyScript, []byte("#!/bin/sh\necho\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveReachableOn(emptyScript); err == nil {
		t.Fatal("empty script output should error")
	}
}

func TestInterfaceDiscoveryHandlerFilters(t *testing.T) {
	tr := transport.NewTransport(nil)
	defer tr.Close()
	var saw int
	d := NewInterfaceDiscovery(tr, DefaultStampValue, func(*ReceivedAnnounceInfo) { saw++ })
	d.Start()
	t.Cleanup(d.Stop)
	if d.handler == nil {
		t.Fatal("handler missing")
	}
	if got := d.handler.AspectFilter(); len(got) != 1 || got[0] != AppName+".discovery.interface" {
		t.Fatalf("AspectFilter = %v", got)
	}
	if d.handler.ReceivePathResponses() {
		t.Fatal("ReceivePathResponses should be false")
	}
	if err := d.handler.ReceivedAnnounce(nil, nil, nil, 0); err != nil {
		t.Fatalf("empty appData: %v", err)
	}
	if saw != 0 {
		t.Fatal("empty announce should not invoke callback")
	}
}

func TestNewInterfaceAnnouncerRequiresArgs(t *testing.T) {
	if _, err := NewInterfaceAnnouncer(nil, &common.ReticulumConfig{}, nil); err == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestAnnounceIntervalClamp(t *testing.T) {
	if got := announceInterval(&common.InterfaceConfig{}); got != DefaultAnnounceInterval {
		t.Fatalf("default interval = %v", got)
	}
	if got := announceInterval(&common.InterfaceConfig{DiscoveryAnnounceIntervalSec: 1}); got != MinAnnounceInterval {
		t.Fatalf("clamp low = %v", got)
	}
	if got := announceInterval(&common.InterfaceConfig{DiscoveryAnnounceIntervalSec: 7200}); got != 2*time.Hour {
		t.Fatalf("custom = %v", got)
	}
}
