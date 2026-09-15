// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
)

var hkdfZeroSalt [32]byte

func implDeriveKeyWrite(dst []byte, secret, salt, info []byte) error {
	length := len(dst)
	if length < 1 {
		return errors.New("invalid output key length")
	}
	if len(secret) == 0 {
		return errors.New("cannot derive key from empty input material")
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

	iterations := (length + 31) / 32
	if iterations > 255 {
		return errors.New("hkdf: output length exceeds maximum")
	}

	var blockBuf [32]byte
	block := blockBuf[:0]
	expand := hmac.New(sha256.New, prk)
	var counter [1]byte
	off := 0
	for i := range iterations {
		expand.Reset()
		expand.Write(block)
		expand.Write(info)
		counter[0] = byte(i + 1)
		expand.Write(counter[:])
		block = expand.Sum(blockBuf[:0])
		n := copy(dst[off:], block)
		off += n
	}
	return nil
}

func implDeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	if length < 1 {
		return nil, errors.New("invalid output key length")
	}
	derived := make([]byte, length)
	if err := implDeriveKeyWrite(derived, secret, salt, info); err != nil {
		return nil, err
	}
	return derived, nil
}

// DeriveKeyInto expands HKDF-SHA256 output into dst. len(dst) must be at least 1.
func DeriveKeyInto(dst []byte, secret, salt, info []byte) error {
	return implDeriveKeyWrite(dst, secret, salt, info)
}

// DeriveKey performs HKDF-SHA256 expansion (non-RFC 5869 extract. Matches legacy use).

func DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	return ActiveProvider().DeriveKey(secret, salt, info, length)
}
