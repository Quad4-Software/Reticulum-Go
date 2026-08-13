// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package pageserver

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

func TestDestPrivateRatchetPathUsesDestHash(t *testing.T) {
	ident, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(ident, destination.Out, destination.Single, "nomadnetwork", nil, "node")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	got := destPrivateRatchetPath(root, dest, ident)
	want := filepath.Join(root, "storage", "ratchets", hex.EncodeToString(dest.GetHash()))
	if got != want {
		t.Fatalf("path = %q, want dest-hash file %q", got, want)
	}
	if ident.GetHexHash() == hex.EncodeToString(dest.GetHash()) {
		t.Fatal("identity hash unexpectedly equals destination hash")
	}
}

func TestDestPrivateRatchetPathRenamesIdentityHashFile(t *testing.T) {
	ident, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(ident, destination.Out, destination.Single, "nomadnetwork", nil, "node")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	dir := filepath.Join(root, "storage", "ratchets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, ident.GetHexHash())
	if err := os.WriteFile(old, []byte("signed-list"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := destPrivateRatchetPath(root, dest, ident)
	want := filepath.Join(dir, hex.EncodeToString(dest.GetHash()))
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("identity-hash ratchet file should have been renamed")
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "signed-list" {
		t.Fatalf("renamed file contents = %q", data)
	}
}
