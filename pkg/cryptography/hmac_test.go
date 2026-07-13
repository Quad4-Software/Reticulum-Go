// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
)

func TestGenerateHMACKey(t *testing.T) {
	testSizes := []int{16, 32, 64}
	for _, size := range testSizes {
		t.Run("Size"+string(rune(size)), func(t *testing.T) { // Simple name conversion
			key, err := GenerateHMACKey(size)
			if err != nil {
				t.Fatalf("GenerateHMACKey(%d) failed: %v", size, err)
			}
			if len(key) != size {
				t.Errorf("GenerateHMACKey(%d) returned key of length %d; want %d", size, len(key), size)
			}

			// Check if key is not all zeros (basic check for randomness)
			isZero := true
			for _, b := range key {
				if b != 0 {
					isZero = false
					break
				}
			}
			if isZero {
				t.Errorf("GenerateHMACKey(%d) returned an all-zero key", size)
			}
		})
	}
}

func TestComputeAndValidateHMAC(t *testing.T) {
	key, err := GenerateHMACKey(32) // Use SHA256 key size
	if err != nil {
		t.Fatalf("Failed to generate HMAC key: %v", err)
	}

	message := []byte("This is a test message.")

	// Compute HMAC
	computedHMAC := ComputeHMAC(key, message)
	if len(computedHMAC) != 32 { // SHA256 output size
		t.Errorf("ComputeHMAC returned HMAC of length %d; want 32", len(computedHMAC))
	}

	// Validate correct HMAC
	if !ValidateHMAC(key, message, computedHMAC) {
		t.Errorf("ValidateHMAC failed for correctly computed HMAC")
	}

	// Validate incorrect HMAC (tampered message)
	tamperedMessage := append(message, byte('!'))
	if ValidateHMAC(key, tamperedMessage, computedHMAC) {
		t.Errorf("ValidateHMAC succeeded for tampered message")
	}

	// Validate incorrect HMAC (tampered key)
	wrongKey, _ := GenerateHMACKey(32)
	if ValidateHMAC(wrongKey, message, computedHMAC) {
		t.Errorf("ValidateHMAC succeeded for incorrect key")
	}

	// Validate incorrect HMAC (tampered HMAC)
	tamperedHMAC := append(computedHMAC[:len(computedHMAC)-1], ^computedHMAC[len(computedHMAC)-1])
	if ValidateHMAC(key, message, tamperedHMAC) {
		t.Errorf("ValidateHMAC succeeded for tampered HMAC")
	}

	// Validate empty message
	emptyMessage := []byte("")
	emptyHMAC := ComputeHMAC(key, emptyMessage)
	if !ValidateHMAC(key, emptyMessage, emptyHMAC) {
		t.Errorf("ValidateHMAC failed for empty message")
	}
	if ValidateHMAC(key, message, emptyHMAC) {
		t.Errorf("ValidateHMAC succeeded comparing non-empty message with empty HMAC")
	}
}

func TestPBTHMACValidate(t *testing.T) {
	keyMsg := pbt.Tuple2(
		"keyMsg",
		byteSliceNonEmpty(64),
		byteSliceMaybeEmpty(2048),
	)
	prop := pbt.ForAll(
		"hmac validate accepts computed mac and rejects tamper",
		keyMsg,
		func(in pbt.Tuple2Value[[]byte, []byte]) bool {
			key := in.First
			msg := in.Second
			mac := ComputeHMAC(key, msg)
			if !ValidateHMAC(key, msg, mac) {
				return false
			}
			if len(msg) > 0 {
				tam := bytes.Clone(msg)
				tam[0] ^= 0x01
				if ValidateHMAC(key, tam, mac) {
					return false
				}
			}
			if len(mac) > 0 {
				bad := bytes.Clone(mac)
				bad[0] ^= 0x01
				if ValidateHMAC(key, msg, bad) {
					return false
				}
			}
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(11))
}
