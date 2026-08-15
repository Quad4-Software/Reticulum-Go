// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/sha256"

	"golang.org/x/crypto/curve25519"
)

const (
	SHA256Size    = 32
	AES256KeySize = 32
	// IdentityKeyMaterialSize is the HKDF output for identity encrypt/decrypt (HMAC key + AES key).
	IdentityKeyMaterialSize = SHA256Size + AES256KeySize
)

func implGetBasepoint() []byte {
	return curve25519.Basepoint
}

func implHash(data []byte) []byte {
	sum := sha256.Sum256(data)
	out := make([]byte, SHA256Size)
	copy(out, sum[:])
	return out
}

// GetBasepoint returns the standard Curve25519 basepoint.
func GetBasepoint() []byte {
	return ActiveProvider().GetBasepoint()
}

// Hash returns the SHA-256 digest of data.
func Hash(data []byte) []byte {
	return ActiveProvider().Hash(data)
}
