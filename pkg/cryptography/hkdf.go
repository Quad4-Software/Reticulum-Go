// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package cryptography

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"math"
)

func DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
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

	iterations := int(math.Ceil(float64(length) / float64(hashLen)))
	for i := 0; i < iterations; i++ {
		h := hmac.New(sha256.New, prk)
		h.Write(block)
		h.Write(info)
		counter := byte((i + 1) % (0xFF + 1))
		h.Write([]byte{counter})
		block = h.Sum(nil)
		derived = append(derived, block...)
	}

	return derived[:length], nil
}
