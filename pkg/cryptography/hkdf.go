// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

var hkdfZeroSalt [32]byte

func implDeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	hashLen := 32

	if length < 1 {
		return nil, errors.New("invalid output key length")
	}

	if len(secret) == 0 {
		return nil, errors.New("cannot derive key from empty input material")
	}

	if len(salt) == 0 {
		salt = hkdfZeroSalt[:]
	}

	if info == nil {
		info = []byte{}
	}

	extract := hmac.New(sha256.New, salt)
	extract.Write(secret)
	prk := extract.Sum(nil)

	iterations := (length + hashLen - 1) / hashLen
	if iterations > 255 {
		return nil, errors.New("hkdf: output length exceeds maximum")
	}

	derived := make([]byte, 0, iterations*hashLen)
	var blockBuf [32]byte
	block := blockBuf[:0]
	expand := hmac.New(sha256.New, prk)
	var counter [1]byte
	for i := range iterations {
		expand.Reset()
		expand.Write(block)
		expand.Write(info)
		counter[0] = byte(i + 1)
		expand.Write(counter[:])
		block = expand.Sum(blockBuf[:0])
		derived = append(derived, block...)
	}

	return derived[:length], nil
}

// DeriveKey performs HKDF-SHA256 expansion (non-RFC 5869 extract. Matches legacy use).

func DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	return ActiveProvider().DeriveKey(secret, salt, info, length)
}
