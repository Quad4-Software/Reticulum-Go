// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEndpointHashStable(t *testing.T) {
	info := &ReceivedAnnounceInfo{Info: Info{ReachableOn: "192.0.2.1", Port: 4242, HasPort: true}}
	a := EndpointHash(info)
	b := EndpointHash(info)
	if !bytes.Equal(a, b) || len(a) != 32 {
		t.Fatalf("hash mismatch or length %d", len(a))
	}
}

func TestLoadPersistedInterfacesRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "discovery", "interfaces")
	if err := os.MkdirAll(store, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "../../../escape"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "not-hex"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("loaded %d unsafe entries", len(list))
	}
}

func TestIsYggIPv6(t *testing.T) {
	if !IsYggIPv6("200::1") {
		t.Fatal("expected ygg")
	}
	if IsYggIPv6("192.0.2.1") {
		t.Fatal("not ygg")
	}
}

func TestPersistLoadDiscoveredInterface(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "peer",
			ReachableOn: "192.0.2.9",
			Port:        7777,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0xab}, 16),
			IFACNetname: "net",
			IFACNetkey:  "key",
		},
		RemoteIdentity: bytes.Repeat([]byte{0xcd}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "discovery", "interfaces")
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		t.Fatalf("persist dir: %v", err)
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("loaded %d", len(list))
	}
	got := list[0]
	if got.Info.Type != info.Info.Type || got.Info.ReachableOn != info.Info.ReachableOn || got.Info.Port != info.Info.Port {
		t.Fatalf("mismatch %+v", got.Info)
	}
}
