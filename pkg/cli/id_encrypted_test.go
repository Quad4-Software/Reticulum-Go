// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity/store"
)

func newTestIdentityFile(t *testing.T) string {
	t.Helper()
	id, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity")
	if err := id.ToFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIDEncryptDecryptRekeyFlow(t *testing.T) {
	path := newTestIdentityFile(t)
	var out, errOut bytes.Buffer
	opts := Options{Stdout: &out, Stderr: &errOut}

	// Encrypt with env-supplied new passphrase (no tty in tests).
	t.Setenv(store.PassphraseEnv, "cli-passphrase-1")
	if code := RunID([]string{"-i", path, "-to-passphrase"}, opts); code != 0 {
		t.Fatalf("to-passphrase exit %d: %s", code, errOut.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.IsEncryptedIdentityPayload(raw) {
		t.Fatal("file not RNE1 after -to-passphrase")
	}

	// Print still works transparently through the encrypted file.
	out.Reset()
	if code := RunID([]string{"-i", path, "-p"}, opts); code != 0 {
		t.Fatalf("print on RNE1 exit %d: %s", code, errOut.String())
	}

	// Headless rekey: old pass from RETICULUM_IDENTITY_PASSPHRASE, new from
	// RETICULUM_IDENTITY_NEW_PASSPHRASE.
	t.Setenv(store.PassphraseEnv, "cli-passphrase-1")
	t.Setenv(store.NewPassphraseEnv, "cli-passphrase-2")
	if code := RunID([]string{"-i", path, "-rekey"}, opts); code != 0 {
		t.Fatalf("rekey exit %d: %s", code, errOut.String())
	}
	os.Unsetenv(store.NewPassphraseEnv)

	// Old pass no longer opens the file.
	t.Setenv(store.PassphraseEnv, "cli-passphrase-1")
	if _, err := store.LoadIdentityBlob(path); err == nil {
		t.Fatal("old passphrase still decrypts after rekey")
	}
	t.Setenv(store.PassphraseEnv, "cli-passphrase-2")
	if _, err := store.LoadIdentityBlob(path); err != nil {
		t.Fatalf("new passphrase does not decrypt after rekey: %v", err)
	}

	// Decrypt back to plaintext.
	if code := RunID([]string{"-i", path, "-to-file"}, opts); code != 0 {
		t.Fatalf("to-file on RNE1 exit %d: %s", code, errOut.String())
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64 {
		t.Fatalf("expected 64-byte plaintext, got %d", len(raw))
	}
}

func TestIDConflictingMigrateFlags(t *testing.T) {
	path := newTestIdentityFile(t)
	var errOut bytes.Buffer
	code := RunID([]string{"-i", path, "-to-passphrase", "-to-file"},
		Options{Stdout: &bytes.Buffer{}, Stderr: &errOut})
	if code != 2 {
		t.Fatalf("conflicting flags exit %d", code)
	}
}
