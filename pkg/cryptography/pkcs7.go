// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/aes"
	"errors"
)

// RemovePKCS7Padding validates and removes PKCS#7 padding without early exit
// on the first mismatched byte (reduces padding-oracle surface when used after MAC verify).
func RemovePKCS7Padding(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("invalid padding: plaintext is empty")
	}

	padding := int(plaintext[len(plaintext)-1])
	if padding > aes.BlockSize || padding == 0 {
		return nil, errors.New("invalid padding size")
	}
	if len(plaintext) < padding {
		return nil, errors.New("invalid padding: padding size is larger than plaintext")
	}

	var bad byte
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		bad |= plaintext[i] ^ byte(padding)
	}
	if bad != 0 {
		return nil, errors.New("invalid padding bytes")
	}

	return plaintext[:len(plaintext)-padding], nil
}
