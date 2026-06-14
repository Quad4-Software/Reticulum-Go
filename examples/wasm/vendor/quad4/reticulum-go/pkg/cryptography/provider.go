// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/ed25519"
	"sync"
)

// CryptoProvider abstracts the cryptographic primitives used by Reticulum.
// The default implementation matches the on-wire protocol (X25519, Ed25519,
// AES-256-CBC, HMAC-SHA256, HKDF-SHA256). Call SetProvider to substitute
// implementations for testing or future algorithm agility; callers must preserve
// wire compatibility unless all peers are upgraded together.
type CryptoProvider interface {
	GenerateKeyPair() (privateKey, publicKey []byte, err error)
	PublicKeyFromPrivate(privateKey []byte) ([]byte, error)
	DeriveSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error)
	GetBasepoint() []byte

	GenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error)
	Sign(privateKey ed25519.PrivateKey, message []byte) []byte
	Verify(publicKey ed25519.PublicKey, message, signature []byte) bool

	EncryptAES256CBC(key, plaintext []byte) ([]byte, error)
	DecryptAES256CBC(key, ciphertext []byte) ([]byte, error)

	ComputeHMAC(key, message []byte) []byte
	ValidateHMAC(key, message, messageHMAC []byte) bool

	Hash(data []byte) []byte
	DeriveKey(secret, salt, info []byte, length int) ([]byte, error)
	ExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error)
	DeriveIdentityKeyMaterial(sharedSecret, salt, context []byte) ([]byte, error)
}

var (
	cryptoProviderMu sync.RWMutex
	activeCrypto     CryptoProvider = stdlibProvider{}
)

// SetProvider replaces the global crypto implementation. Passing nil restores
// the default stdlib-backed provider.
func SetProvider(p CryptoProvider) {
	if p == nil {
		p = stdlibProvider{}
	}
	cryptoProviderMu.Lock()
	activeCrypto = p
	cryptoProviderMu.Unlock()
}

// ActiveProvider returns the current CryptoProvider (for tests or advanced use).
func ActiveProvider() CryptoProvider {
	cryptoProviderMu.RLock()
	defer cryptoProviderMu.RUnlock()
	return activeCrypto
}
