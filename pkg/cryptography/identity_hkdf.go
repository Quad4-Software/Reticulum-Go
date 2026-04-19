// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

// implDeriveIdentityKeyMaterial derives 64 bytes (HMAC key + AES-256 key) via
// RFC 5869 HKDF-SHA256, matching Identity Encrypt/Decrypt.
func implDeriveIdentityKeyMaterial(sharedSecret, salt, context []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, sharedSecret, salt, context)
	key := make([]byte, IdentityKeyMaterialSize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

// DeriveIdentityKeyMaterial derives 64 bytes of key material for identity-layer
// encryption using the active CryptoProvider.
func DeriveIdentityKeyMaterial(sharedSecret, salt, context []byte) ([]byte, error) {
	return ActiveProvider().DeriveIdentityKeyMaterial(sharedSecret, salt, context)
}
