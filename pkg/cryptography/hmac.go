// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
)

func GenerateHMACKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func implComputeHMAC(key, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

func implValidateHMAC(key, message, messageHMAC []byte) bool {
	expectedHMAC := implComputeHMAC(key, message)
	return hmac.Equal(messageHMAC, expectedHMAC)
}

// ComputeHMAC returns HMAC-SHA256(key, message).
func ComputeHMAC(key, message []byte) []byte {
	return ActiveProvider().ComputeHMAC(key, message)
}

// ValidateHMAC performs a constant-time comparison of the MAC.
func ValidateHMAC(key, message, messageHMAC []byte) bool {
	return ActiveProvider().ValidateHMAC(key, message, messageHMAC)
}
