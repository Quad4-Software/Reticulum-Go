// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package cryptography

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

// encryptWithHMACInfo is a fixed domain separator for HKDF expansion of 32-byte
// GROUP/ratchet keys into 64 bytes (HMAC key + AES-256 key).
var encryptWithHMACInfo = []byte("identity.EncryptWithHMAC.v1")

// ExpandEncryptWithHMACKeyMaterial derives 32-byte HMAC and 32-byte AES keys from a
// 32-byte input using HKDF-SHA256 (RFC 5869).
func ExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error) {
	salt := make([]byte, SHA256Size)
	r := hkdf.New(sha256.New, key32, salt, encryptWithHMACInfo)
	out := make([]byte, 64)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, nil, err
	}
	return out[:32], out[32:], nil
}
