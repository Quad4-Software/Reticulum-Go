// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyringBackendRoundTrip(t *testing.T) {
	if os.Getenv("RETICULUM_TEST_KEYRING") != "1" {
		t.Skip("set RETICULUM_TEST_KEYRING=1 to exercise live keyctl")
	}
	b, err := NewKeyringBackend()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "id")
	secret := bytes.Repeat([]byte{0x7a}, 64)
	attrs := AttrsForPath(path, "transport")
	if err := b.Set(attrs, secret, ""); err != nil {
		t.Fatal(err)
	}
	got, err := b.Get(attrs)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("mismatch")
	}
	if err := b.Delete(attrs); err != nil {
		t.Fatal(err)
	}
}

func TestKeyringMarkerViaActiveBackend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id")
	secret := bytes.Repeat([]byte{0x3c}, 64)

	mem := NewMemoryBackend()
	prevName := BackendName()
	prev := Active()
	t.Cleanup(func() { SetActiveBackend(prevName, prev) })
	SetActiveBackend(BackendKeyring, mem)

	if err := SaveIdentityBlob(path, secret, "app"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsMarkerPayload(raw) {
		t.Fatal("expected RSSI marker")
	}
	got, err := LoadIdentityBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("load mismatch")
	}
}
