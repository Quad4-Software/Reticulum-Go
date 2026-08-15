// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkerRoundTripWithMemoryBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transport_identity")
	secret := bytes.Repeat([]byte{0x42}, 64)

	mem := NewMemoryBackend()
	prevName := BackendName()
	prev := Active()
	t.Cleanup(func() {
		SetActiveBackend(prevName, prev)
	})
	SetActiveBackend(BackendSecretService, mem)

	if err := SaveIdentityBlob(path, secret, "transport"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsMarkerPayload(raw) {
		t.Fatalf("expected RSSI marker, got %d bytes", len(raw))
	}

	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("loaded secret mismatch")
	}
}

func TestFileBackendPlain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id")
	secret := bytes.Repeat([]byte{0x11}, 64)

	prevName := BackendName()
	prev := Active()
	t.Cleanup(func() {
		SetActiveBackend(prevName, prev)
	})
	SetActiveBackend(BackendFile, FileBackend{})

	if err := SaveIdentityBlob(path, secret, ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if IsMarkerPayload(raw) {
		t.Fatal("file backend should not write marker")
	}
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("mismatch")
	}
}

func TestSetBackendNameUnknown(t *testing.T) {
	if err := SetBackendName("nope"); err == nil {
		t.Fatal("expected error")
	}
}
