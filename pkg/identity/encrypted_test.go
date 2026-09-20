// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity/store"
)

func TestToEncryptedFileRoundTrip(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity")
	pass := []byte("test-passphrase-1")
	if err := id.ToEncryptedFile(path, pass); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedIdentityFilePayload(raw) {
		t.Fatal("file is not RNE1")
	}
	enc, err := IdentityFileIsEncrypted(path)
	if err != nil || !enc {
		t.Fatal("IdentityFileIsEncrypted false on RNE1 file")
	}
	t.Setenv(store.PassphraseEnv, string(pass))
	got, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Hash(), id.Hash()) {
		t.Fatal("encrypted identity hash mismatch")
	}
}

func TestEncryptedFileHelpers(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := id.ToFile(path); err != nil {
		t.Fatal(err)
	}
	pass := []byte("helper-passphrase")
	if err := EncryptIdentityFile(path, pass); err != nil {
		t.Fatal(err)
	}
	t.Setenv(store.PassphraseEnv, string(pass))
	got, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Hash(), id.Hash()) {
		t.Fatal("hash changed through encrypt cycle")
	}
	if err := RekeyIdentityFile(path, pass, []byte("helper-pass-2")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(store.PassphraseEnv, "helper-pass-2")
	if err := DecryptIdentityFile(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64 {
		t.Fatalf("decrypt did not restore 64-byte file, got %d bytes", len(raw))
	}
	got, err = FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Hash(), id.Hash()) {
		t.Fatal("hash changed through decrypt cycle")
	}
}

func TestTransportIdentityNotReplacedOnUnlockFailure(t *testing.T) {
	dir := t.TempDir()
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "transport_identity")
	pass := []byte("transport-pass-1")
	if err := id.ToEncryptedFile(path, pass); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// No passphrase available: load must fail and the file must survive.
	t.Setenv(store.PassphraseEnv, "wrong-pass-xxx")
	if _, err := LoadOrCreateTransportIdentity(dir); err == nil {
		t.Fatal("encrypted transport identity silently replaced")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("RNE1 file modified by failed unlock")
	}
	// With the right passphrase it loads.
	t.Setenv(store.PassphraseEnv, string(pass))
	got, err := LoadOrCreateTransportIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Hash(), id.Hash()) {
		t.Fatal("transport identity hash mismatch")
	}
}

func TestTransportIdentityUnreadableNotReplaced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "transport_identity")
	if err := os.WriteFile(path, testBlob64(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := LoadOrCreateTransportIdentity(dir); err == nil {
		t.Fatal("unreadable transport identity silently replaced")
	}
}

func testBlob64() []byte {
	return bytes.Repeat([]byte{0x42}, 64)
}

func TestEncryptedIdentityErrorHint(t *testing.T) {
	err := EncryptedIdentityError(store.ErrPassphraseRequired)
	if err == nil {
		t.Fatal("nil error for passphrase-required")
	}
	if EncryptedIdentityError(nil) != nil {
		t.Fatal("non-nil for nil input")
	}
}
