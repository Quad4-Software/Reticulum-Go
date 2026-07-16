// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateTransportIdentity_EmptyPathIsEphemeral(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	ident, err := LoadOrCreateTransportIdentity("")
	if err != nil {
		t.Fatalf("LoadOrCreateTransportIdentity: %v", err)
	}
	if ident == nil {
		t.Fatal("expected identity")
	}

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == ".reticulum" || e.Name() == ".reticulum-go" {
			t.Fatalf("ephemeral identity wrote under home: %s", e.Name())
		}
	}
}

func TestLoadOrCreateTransportIdentity_PersistsWhenPathSet(t *testing.T) {
	dir := t.TempDir()
	ident, err := LoadOrCreateTransportIdentity(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	path := filepath.Join(dir, "transport_identity")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected transport_identity on disk: %v", err)
	}
	loaded, err := LoadOrCreateTransportIdentity(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if string(ident.Hash()) != string(loaded.Hash()) {
		t.Fatal("reloaded identity hash mismatch")
	}
}

func TestKnownDestinationsEvictionUnderCap(t *testing.T) {
	resetKnownDestinations(t)
	t.Cleanup(func() { SetKnownDestinationsMaxEntries(0) })
	SetKnownDestinationsMaxEntries(3)

	for i := range 5 {
		h := make([]byte, TruncatedHashLength/8)
		h[0] = byte(i + 1)
		pk := make([]byte, KeySize/8)
		pk[0] = byte(i + 1)
		Remember([]byte("p"), h, pk, []byte("a"))
	}

	knownDestinationsLock.RLock()
	n := len(knownDestinations)
	knownDestinationsLock.RUnlock()
	if n != 3 {
		t.Fatalf("known destinations = %d, want 3 after eviction", n)
	}
}

func TestKnownDestinationsCapDisabled(t *testing.T) {
	resetKnownDestinations(t)
	SetKnownDestinationsMaxEntries(0)
	for i := range 10 {
		h := make([]byte, TruncatedHashLength/8)
		h[0] = byte(i + 1)
		pk := make([]byte, KeySize/8)
		pk[0] = byte(i + 1)
		Remember([]byte("p"), h, pk, []byte("a"))
	}
	knownDestinationsLock.RLock()
	n := len(knownDestinations)
	knownDestinationsLock.RUnlock()
	if n != 10 {
		t.Fatalf("known destinations = %d, want 10", n)
	}
}
