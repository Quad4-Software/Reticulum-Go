// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
)

type persistOracle struct {
	Files     int
	Loaded    int
	Filename  string
	Transport bool
}

func TestOraclePersistLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "TCPServerInterface",
			Name:        "oracle-tcp",
			ReachableOn: "192.0.2.77",
			Port:        5151,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0x41}, 16),
			IFACNetname: "net",
			IFACNetkey:  "key",
		},
		RemoteIdentity: bytes.Repeat([]byte{0x42}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	oracle := persistOracle{
		Filename:  hex.EncodeToString(DiscoveryHash(info.Info.TransportID, info.Info.Name)),
		Transport: info.Info.Transport,
	}
	store := filepath.Join(dir, "discovery", "interfaces")
	entries, err := os.ReadDir(store)
	if err != nil {
		t.Fatal(err)
	}
	oracle.Files = len(entries)
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	oracle.Loaded = len(list)
	if oracle.Files != 1 || oracle.Loaded != 1 {
		t.Fatalf("oracle files=%d loaded=%d", oracle.Files, oracle.Loaded)
	}
	got := list[0]
	if got.Info.Type != info.Info.Type || got.Info.ReachableOn != info.Info.ReachableOn || got.Info.Port != info.Info.Port {
		t.Fatalf("roundtrip mismatch %+v", got.Info)
	}
	if !bytes.Equal(got.RemoteIdentity, info.RemoteIdentity) {
		t.Fatal("network_id mismatch")
	}
	if !got.Info.Transport {
		t.Fatal("transport flag lost")
	}
	if oracle.Filename != entries[0].Name() {
		t.Fatalf("filename %q want %q", entries[0].Name(), oracle.Filename)
	}
}

func TestOracleDiscoveryHashFilenameMatchesPersist(t *testing.T) {
	dir := t.TempDir()
	tid := bytes.Repeat([]byte{0xab}, 16)
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "bb-peer",
			ReachableOn: "198.51.100.2",
			Port:        7777,
			HasPort:     true,
			TransportID: tid,
		},
		RemoteIdentity: bytes.Repeat([]byte{0xcd}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	wantName := hex.EncodeToString(DiscoveryHash(tid, info.Info.Name))
	path := filepath.Join(dir, "discovery", "interfaces", wantName)
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		t.Fatalf("persist path: %v", err)
	}
}

func TestOracleLoadIgnoresCorruptPersistBlob(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "discovery", "interfaces")
	if err := os.MkdirAll(store, 0o750); err != nil {
		t.Fatal(err)
	}
	valid := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "BackboneInterface",
			Name:        "valid",
			ReachableOn: "192.0.2.3",
			Port:        1,
			HasPort:     true,
			Transport:   true,
			TransportID: bytes.Repeat([]byte{0x01}, 16),
		},
		RemoteIdentity: bytes.Repeat([]byte{0x02}, 16),
	}
	if err := PersistDiscoveredInterface(dir, valid); err != nil {
		t.Fatal(err)
	}
	corruptName := hex.EncodeToString(bytes.Repeat([]byte{0xcc}, 32))
	if err := os.WriteFile(filepath.Join(store, corruptName), []byte("not-msgpack"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := LoadPersistedInterfaces(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("loaded %d want 1 valid entry", len(list))
	}
}

func TestOraclePersistMsgpackFields(t *testing.T) {
	dir := t.TempDir()
	tid := bytes.Repeat([]byte{0x55}, 16)
	info := &ReceivedAnnounceInfo{
		Info: Info{
			Type:        "I2PInterface",
			Name:        "i2p-oracle",
			ReachableOn: "abc.b32.i2p",
			Transport:   true,
			TransportID: tid,
		},
		RemoteIdentity: bytes.Repeat([]byte{0x77}, 16),
	}
	if err := PersistDiscoveredInterface(dir, info); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "discovery", "interfaces", hex.EncodeToString(DiscoveryHash(tid, info.Info.Name))))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := msgpack.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "I2PInterface" || m["reachable_on"] != "abc.b32.i2p" {
		t.Fatalf("msgpack map=%v", m)
	}
}
