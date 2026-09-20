// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package store

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
)

// wrapKind distinguishes passphrase-wrap entries from identity-secret entries
// in shared backends.
const wrapKind = "passphrase"

type wrapCandidate struct {
	name string
	open func() (Backend, error)
}

// wrapBackendCandidates lists the platform's wrap stores in preference order.
var wrapBackendCandidates = defaultWrapCandidates()

func wrapAttrs(path string) (map[string]string, error) {
	abs, err := AbsolutePath(path)
	if err != nil {
		return nil, err
	}
	return AttrsForPath(abs, wrapKind), nil
}

// wrapAccount derives a stable per-item account name for CLI-style backends
// (macOS security, others) that key items by service plus account strings.
func wrapAccount(attrs map[string]string) string {
	return attrs[AttrIdentityKind] + "|" + attrs[AttrIdentityPath]
}

// dropMarkerBackendEntries best-effort removes an orphaned secret-store entry
// after a marker-backed identity file is rewritten to another format.
func dropMarkerBackendEntries(path string) {
	abs, err := AbsolutePath(path)
	if err != nil {
		return
	}
	attrs := AttrsForPath(abs, "")
	if kr, err := keyringFactory(); err == nil {
		_ = kr.Delete(attrs)
	}
	if ss, err := ssFactory(); err == nil {
		_ = ss.Delete(attrs)
	}
}

// resolveWrapPassphrase looks up a stored wrap passphrase for path.
func resolveWrapPassphrase(path string) ([]byte, error) {
	attrs, err := wrapAttrs(path)
	if err != nil {
		return nil, err
	}
	for _, c := range wrapBackendCandidates {
		b, err := c.open()
		if err != nil {
			continue
		}
		if p, err := b.Get(attrs); err == nil && len(p) > 0 {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

// storeWrapPassphrase saves a wrap passphrase in the first available backend.
func storeWrapPassphrase(path string, passphrase []byte) (string, error) {
	attrs, err := wrapAttrs(path)
	if err != nil {
		return "", err
	}
	for _, c := range wrapBackendCandidates {
		b, err := c.open()
		if err != nil {
			continue
		}
		if err := b.Set(attrs, passphrase, "Reticulum-Go identity passphrase"); err == nil {
			return c.name, nil
		}
	}
	return "", ErrUnsupported
}

// deleteWrapPassphrase removes any stored wrap passphrase for path.
func deleteWrapPassphrase(path string) {
	attrs, err := wrapAttrs(path)
	if err != nil {
		return
	}
	for _, c := range wrapBackendCandidates {
		if b, err := c.open(); err == nil {
			_ = b.Delete(attrs)
		}
	}
}

// MigrateToPassphrase rewrites the identity file at path as an RNE1 envelope
// protected by passphrase. The plaintext blob is read first, so marker-backed
// identities can be converted too.
func MigrateToPassphrase(path string, passphrase []byte) error {
	raw, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	if IsEncryptedIdentityPayload(raw) {
		return fmt.Errorf("identity file already passphrase encrypted")
	}
	wasMarker := IsMarkerPayload(raw)
	data, err := LoadIdentityBlob(path)
	if err != nil {
		return err
	}
	defer securemem.WipeBytes(data)
	if len(data) != 64 {
		return fmt.Errorf("identity file does not contain a 64-byte software identity (got %d bytes)", len(data))
	}
	rne1, err := EncryptIdentityPayload(data, passphrase)
	if err != nil {
		return err
	}
	if err := storage.AtomicWriteFile(path, rne1, 0o600); err != nil {
		return err
	}
	// A passphrase-mode file must not keep a stale wrap secret: a leftover
	// entry would resolve before the prompt and fail decryption.
	deleteWrapPassphrase(path)
	if wasMarker {
		dropMarkerBackendEntries(path)
	}
	return nil
}

// MigrateToWrapped encrypts the identity file under a fresh random passphrase
// and stores that passphrase in the platform wrap backend, giving encrypted
// at rest with unattended unlock. Returns the backend name used.
func MigrateToWrapped(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return "", err
	}
	if IsEncryptedIdentityPayload(raw) {
		return "", fmt.Errorf("identity file already passphrase encrypted")
	}
	wasMarker := IsMarkerPayload(raw)
	data, err := LoadIdentityBlob(path)
	if err != nil {
		return "", err
	}
	defer securemem.WipeBytes(data)
	if len(data) != 64 {
		return "", fmt.Errorf("identity file does not contain a 64-byte software identity (got %d bytes)", len(data))
	}
	pass := make([]byte, 32)
	if _, err := rand.Read(pass); err != nil {
		return "", err
	}
	defer securemem.WipeBytes(pass)
	rne1, err := EncryptIdentityPayload(data, pass)
	if err != nil {
		return "", err
	}
	name, err := storeWrapPassphrase(path, pass)
	if err != nil {
		return "", err
	}
	if err := storage.AtomicWriteFile(path, rne1, 0o600); err != nil {
		deleteWrapPassphrase(path)
		return "", err
	}
	if wasMarker {
		dropMarkerBackendEntries(path)
	}
	return name, nil
}

// MigrateEncryptedToFile decrypts an RNE1 identity file back to plaintext,
// removing any stored wrap passphrase.
func MigrateEncryptedToFile(path string) error {
	raw, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	if !IsEncryptedIdentityPayload(raw) {
		return fmt.Errorf("identity file is not passphrase encrypted")
	}
	data, err := LoadIdentityBlob(path)
	if err != nil {
		return err
	}
	defer securemem.WipeBytes(data)
	if len(data) != 64 {
		return fmt.Errorf("decrypted payload is not a 64-byte software identity (got %d bytes)", len(data))
	}
	if err := storage.AtomicWriteFile(path, data, 0o600); err != nil {
		return err
	}
	deleteWrapPassphrase(path)
	dropMarkerBackendEntries(path)
	return nil
}

// RekeyEncryptedFile changes the passphrase on an RNE1 identity file. A stored
// wrap passphrase is dropped; run MigrateToWrapped again to restore it.
func RekeyEncryptedFile(path string, oldPass, newPass []byte) error {
	raw, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return err
	}
	if !IsEncryptedIdentityPayload(raw) {
		return fmt.Errorf("identity file is not passphrase encrypted")
	}
	rne1, err := RekeyIdentityPayload(raw, oldPass, newPass)
	if err != nil {
		return err
	}
	if err := storage.AtomicWriteFile(path, rne1, 0o600); err != nil {
		return err
	}
	deleteWrapPassphrase(path)
	return nil
}

// sidecarPath is where the DPAPI backend keeps its protected blob.
func sidecarPath(path string) string {
	return filepath.Clean(path) + ".dpwrap"
}
