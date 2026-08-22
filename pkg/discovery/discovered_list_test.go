// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

func TestDiscoveryHashMatchesPythonMaterial(t *testing.T) {
	tid := make([]byte, 16)
	for i := range tid {
		tid[i] = byte(i)
	}
	name := "test-node"
	h := DiscoveryHash(tid, name)
	if len(h) != 32 {
		t.Fatalf("len=%d", len(h))
	}
	material := hex.EncodeToString(tid) + name
	want := sha256.Sum256([]byte(material))
	if string(h) != string(want[:]) {
		t.Fatal("hash mismatch")
	}
}

func TestPersistAndListDiscoveredInterface(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		StampValue: 42,
		Hops:       2,
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "peer-a",
			Transport:   true,
			ReachableOn: "192.0.2.1",
			Port:        4242,
			HasPort:     true,
			TransportID: bytes16(0x11),
		},
		RemoteIdentity: bytes16(0x22),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	list, err := ListDiscoveredInterfaces(dir, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].Name != "peer-a" || list[0].Hops != 2 || list[0].Value != 42 {
		t.Fatalf("record=%+v", list[0])
	}
	if list[0].Status != statusAvailable {
		t.Fatalf("status=%s", list[0].Status)
	}
}

func TestListDiscoveredRemovesStale(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "discovery", "interfaces")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour).Unix()
	payload := map[string]any{
		"type":         "BackboneInterface",
		"name":         "old",
		"transport":    true,
		"transport_id": hex.EncodeToString(bytes16(1)),
		"network_id":   hex.EncodeToString(bytes16(2)),
		"reachable_on": "192.0.2.2",
		"port":         int64(4242),
		"last_heard":   float64(old),
		"discovered":   float64(old),
		"value":        10,
		"hops":         1,
	}
	raw, err := msgpack.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, hex.EncodeToString(bytes16(0xab))), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := ListDiscoveredInterfaces(dir, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected removal, got %+v", list)
	}
}

func TestLoadPersistedInterfacesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "I2PInterface",
			Name:        "i2p-peer",
			ReachableOn: "abc.b32.i2p",
			TransportID: bytes16(0x33),
		},
		RemoteIdentity: bytes16(0x44),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Info.Name != "i2p-peer" {
		t.Fatalf("list=%+v", list)
	}
}

func TestListDiscoveredNameFilter(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		info := &ReceivedAnnounceInfo{
			Info: Info{
				Type:        "BackboneInterface",
				Name:        name,
				ReachableOn: "192.0.2.3",
				Port:        1,
				HasPort:     true,
				TransportID: bytes16(byte(name[0])),
			},
			RemoteIdentity: bytes16(0x55),
		}
		if err := PersistDiscoveredInterface(dir, info); err != nil {
			t.Fatal(err)
		}
	}
	list, err := ListDiscoveredInterfaces(dir, ListOptions{NameFilter: "alp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "alpha" {
		t.Fatalf("list=%+v", list)
	}
}

func bytes16(v byte) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = v
	}
	return b
}
