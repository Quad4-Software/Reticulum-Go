// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/identity/store"
)

func TestToFileFromFileSecretServiceMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id")

	mem := store.NewMemoryBackend()
	prevName := store.BackendName()
	prev := store.Active()
	t.Cleanup(func() {
		store.SetActiveBackend(prevName, prev)
	})
	store.SetActiveBackend(store.BackendSecretService, mem)

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer id.Close()
	want := id.GetHexHash()
	if err := id.ToFile(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsMarkerPayload(raw) {
		t.Fatalf("expected RSSI marker, got %d bytes", len(raw))
	}
	loaded, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if loaded.GetHexHash() != want {
		t.Fatalf("hash %s vs %s", loaded.GetHexHash(), want)
	}
}
