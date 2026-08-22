// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestAdversarialEndpointHashNilInfo(t *testing.T) {
	if got := EndpointHash(nil); got != nil {
		t.Fatalf("nil info hash=%v want nil", got)
	}
}

func TestAdversarialPersistNilAndEmptyStorage(t *testing.T) {
	if err := PersistDiscoveredInterface("", &ReceivedAnnounceInfo{
		Info: Info{Type: "BackboneInterface", ReachableOn: "192.0.2.1", HasPort: true, Port: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := PersistDiscoveredInterface("/tmp", nil); err != nil {
		t.Fatal(err)
	}
}

func TestAdversarialLoadPersistedGarbageFiles(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "discovery", "interfaces")
	if err := os.MkdirAll(store, 0o750); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		content []byte
	}{
		{"shorthex", []byte("x")},
		{"validhex-missing-fields", nil},
		{"validhex-empty-map", []byte{0x80}},
	}
	validHex := hex.EncodeToString(bytes.Repeat([]byte{0xab}, 32))
	cases[1].name = validHex
	cases[2].name = hex.EncodeToString(bytes.Repeat([]byte{0xcd}, 32))
	for _, tc := range cases {
		path := filepath.Join(store, tc.name)
		if err := os.WriteFile(path, tc.content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("loaded %d from garbage store", len(list))
	}
}

func TestAdversarialPersistMissingReachableOn(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{Info: Info{Type: "BackboneInterface", Name: "no-host"}}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("loaded %d without reachable_on", len(list))
	}
}

func TestAdversarialIsYggIPv6Inputs(t *testing.T) {
	cases := []struct {
		addr string
		ygg  bool
	}{
		{"", false},
		{"not-an-ip", false},
		{"::1", false},
		{"fe80::1", false},
		{"201::1", true},
		{"200::1", true},
		{"192.0.2.1", false},
	}
	for _, tc := range cases {
		if got := IsYggIPv6(tc.addr); got != tc.ygg {
			t.Fatalf("%q ygg=%v want %v", tc.addr, got, tc.ygg)
		}
	}
}
