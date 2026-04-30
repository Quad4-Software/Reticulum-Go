// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"path/filepath"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
)

func TestHardwareBoundRoundTripMatchesSoftwareIdentity(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := cryptography.NewSoftwareEd25519Signer(pk[32:])
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	hbPath := filepath.Join(dir, "identity")
	if err := id.ToHardwareBoundFile(hbPath); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadIdentityFile(hbPath, signer)
	if err != nil {
		t.Fatal(err)
	}
	if id.GetHexHash() != loaded.GetHexHash() {
		t.Fatalf("hash mismatch: %s vs %s", id.GetHexHash(), loaded.GetHexHash())
	}

	plain := []byte("hello-reticulum")
	ct, err := id.Encrypt(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := loaded.Decrypt(ct, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Fatalf("decrypt mismatch: %q vs %q", out, plain)
	}
}

func TestLoadIdentityFileSoftwareStill64Bytes(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "id")
	if err := id.ToFile(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadIdentityFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id.GetHexHash() != loaded.GetHexHash() {
		t.Fatal("hash mismatch")
	}
}

func TestHardwareBoundWrongSigner(t *testing.T) {
	id, _ := NewIdentity()
	other, _ := NewIdentity()
	pkOther, err := other.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner, err := cryptography.NewSoftwareEd25519Signer(pkOther[32:])
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := id.ToHardwareBoundFile(path); err != nil {
		t.Fatal(err)
	}
	_, err = LoadIdentityFile(path, wrongSigner)
	if err != ErrHardwareBoundSignerPublicKeyMismatch {
		t.Fatalf("want %v, got %v", ErrHardwareBoundSignerPublicKeyMismatch, err)
	}
}

func TestHardwareBoundNilSignerNoHook(t *testing.T) {
	id, _ := NewIdentity()
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := id.ToHardwareBoundFile(path); err != nil {
		t.Fatal(err)
	}
	prev := OptionalIdentitySignerHook
	OptionalIdentitySignerHook = nil
	t.Cleanup(func() { OptionalIdentitySignerHook = prev })
	_, err := LoadIdentityFile(path, nil)
	if err != ErrHardwareBoundSignerRequired {
		t.Fatalf("want %v, got %v", ErrHardwareBoundSignerRequired, err)
	}
}

func TestHardwareBoundOptionalHook(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := cryptography.NewSoftwareEd25519Signer(pk[32:])
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := id.ToHardwareBoundFile(path); err != nil {
		t.Fatal(err)
	}

	prev := OptionalIdentitySignerHook
	OptionalIdentitySignerHook = func() (cryptography.Ed25519Signer, error) {
		return signer, nil
	}
	t.Cleanup(func() { OptionalIdentitySignerHook = prev })

	loaded, err := LoadIdentityFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetHexHash() != id.GetHexHash() {
		t.Fatal("hash mismatch")
	}
}
