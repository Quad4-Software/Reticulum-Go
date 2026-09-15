// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/aes"
	"errors"
)

var (
	errPaddingEmpty    = errors.New("invalid padding: plaintext is empty")
	errPaddingSize     = errors.New("invalid padding size")
	errPaddingTooLarge = errors.New("invalid padding: padding size is larger than plaintext")
	errPaddingBytes    = errors.New("invalid padding bytes")
)

// IsPaddingError reports whether err is a PKCS#7 padding validation failure.
func IsPaddingError(err error) bool {
	return errors.Is(err, errPaddingEmpty) ||
		errors.Is(err, errPaddingSize) ||
		errors.Is(err, errPaddingTooLarge) ||
		errors.Is(err, errPaddingBytes)
}

// RemovePKCS7Padding validates and removes PKCS#7 padding without early exit
// on the first mismatched byte (reduces padding-oracle surface when used after MAC verify).
// plaintext must be a non-empty multiple of AES block size.
func RemovePKCS7Padding(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errPaddingEmpty
	}
	if len(plaintext)%aes.BlockSize != 0 {
		return nil, errPaddingSize
	}

	padding := int(plaintext[len(plaintext)-1])
	if padding > aes.BlockSize || padding == 0 {
		return nil, errPaddingSize
	}
	if len(plaintext) < padding {
		return nil, errPaddingTooLarge
	}

	var bad byte
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		bad |= plaintext[i] ^ byte(padding)
	}
	if bad != 0 {
		return nil, errPaddingBytes
	}

	return plaintext[:len(plaintext)-padding], nil
}
