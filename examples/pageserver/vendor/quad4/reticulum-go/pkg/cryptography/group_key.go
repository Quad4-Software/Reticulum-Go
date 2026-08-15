// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

var encryptWithHMACInfo = []byte("identity.EncryptWithHMAC.v1")

func implExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error) {
	salt := make([]byte, SHA256Size)
	r := hkdf.New(sha256.New, key32, salt, encryptWithHMACInfo)
	out := make([]byte, IdentityKeyMaterialSize)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, nil, err
	}
	return out[:32], out[32:], nil
}

// ExpandEncryptWithHMACKeyMaterial derives 32-byte HMAC and 32-byte AES keys from a
// 32-byte input using HKDF-SHA256 (RFC 5869).
func ExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error) {
	return ActiveProvider().ExpandEncryptWithHMACKeyMaterial(key32)
}
