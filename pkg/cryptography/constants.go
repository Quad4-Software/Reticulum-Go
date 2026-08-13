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
	// TokenKeySize is the default GROUP/Token key (32 HMAC + 32 AES-256).
	TokenKeySize = 64
	// TokenKeySize128 is the AES-128 Token key (16 HMAC + 16 AES).
	TokenKeySize128 = 32
	// TokenOverhead is IV (16) plus HMAC-SHA256 (32).
	TokenOverhead = 48
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
