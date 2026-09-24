// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"errors"
	"fmt"
	"os"

	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity/store"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
)

// IsEncryptedIdentityFilePayload reports whether data is an RNE1 v1
// passphrase-encrypted identity envelope.
func IsEncryptedIdentityFilePayload(data []byte) bool {
	return store.IsEncryptedIdentityPayload(data)
}

// SetPassphraseResolver installs the resolver consulted when an RNE1
// identity file is loaded. Embedders (librns, apps) use this to supply a
// passphrase UI. Nil restores the default env/fd/backend/prompt chain.
func SetPassphraseResolver(r store.PassphraseResolver) {
	store.SetPassphraseResolver(r)
}

// ToEncryptedFile saves the identity as an RNE1 passphrase-encrypted file.
// Requires exportable key material; fails for hardware-bound identities.
func (i *Identity) ToEncryptedFile(path string, passphrase []byte) error {
	if i.externalSigner != nil {
		return ErrSigningMaterialNotExportable
	}
	if !i.hasExportablePrivate() {
		return errors.New("cannot save identity without private keys")
	}
	privateKeyBytes := make([]byte, 64)
	copy(privateKeyBytes[:32], i.privateKey.Bytes())
	copy(privateKeyBytes[32:], i.signingSeed.Bytes())
	defer securemem.WipeBytes(privateKeyBytes)
	rne1, err := store.EncryptIdentityPayload(privateKeyBytes, passphrase)
	if err != nil {
		return err
	}
	return storage.AtomicWriteFile(path, rne1, 0o600)
}

// EncryptIdentityFile rewrites the identity file at path as an RNE1
// passphrase-encrypted envelope.
func EncryptIdentityFile(path string, passphrase []byte) error {
	return store.MigrateToPassphrase(path, passphrase)
}

// WrapIdentityFile encrypts the identity file under a random passphrase held
// by the platform credential store (kernel keyring or Secret Service on
// Linux, Keychain on macOS, DPAPI sidecar on Windows). Returns the backend
// name used. Unsupported platforms return store.ErrUnsupported.
func WrapIdentityFile(path string) (string, error) {
	return store.MigrateToWrapped(path)
}

// DecryptIdentityFile converts an RNE1 identity file back to the standard
// plaintext format.
func DecryptIdentityFile(path string) error {
	return store.MigrateEncryptedToFile(path)
}

// RekeyIdentityFile changes the passphrase on an RNE1 identity file.
func RekeyIdentityFile(path string, oldPass, newPass []byte) error {
	return store.RekeyEncryptedFile(path, oldPass, newPass)
}

// IdentityFileIsEncrypted reports whether the file at path is an RNE1 envelope.
func IdentityFileIsEncrypted(path string) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-chosen identity path
	if err != nil {
		return false, err
	}
	return IsEncryptedIdentityFilePayload(data), nil
}

// EncryptedIdentityError describes a load that needs a passphrase.
func EncryptedIdentityError(err error) error {
	if errors.Is(err, store.ErrPassphraseRequired) {
		return fmt.Errorf("%w (set %s, use a wrap backend, or run on a terminal)", err, store.PassphraseEnv)
	}
	return err
}
