// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

func implDeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	hashLen := 32

	if length < 1 {
		return nil, errors.New("invalid output key length")
	}

	if len(secret) == 0 {
		return nil, errors.New("cannot derive key from empty input material")
	}

	if len(salt) == 0 {
		salt = make([]byte, hashLen)
	}

	if info == nil {
		info = []byte{}
	}

	pseudorandomKey := hmac.New(sha256.New, salt)
	pseudorandomKey.Write(secret)
	prk := pseudorandomKey.Sum(nil)

	block := []byte{}
	derived := []byte{}

	iterations := (length + hashLen - 1) / hashLen
	if iterations > 255 {
		return nil, errors.New("hkdf: output length exceeds maximum")
	}
	for i := range iterations {
		h := hmac.New(sha256.New, prk)
		h.Write(block)
		h.Write(info)
		counter := byte(i + 1)
		h.Write([]byte{counter})
		block = h.Sum(nil)
		derived = append(derived, block...)
	}

	return derived[:length], nil
}

// DeriveKey performs HKDF-SHA256 expansion (non-RFC 5869 extract; matches legacy use).
func DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	return ActiveProvider().DeriveKey(secret, salt, info, length)
}
