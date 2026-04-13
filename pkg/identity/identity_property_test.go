// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package identity

import (
	"bytes"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
)

func FuzzIdentitySignVerify(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		id, err := New()
		if err != nil {
			t.Fatalf("Failed to create identity: %v", err)
		}

		sig, err := id.Sign(data)
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		if !id.Verify(data, sig) {
			t.Error("Verification failed for valid signature")
		}

		if len(data) > 0 {
			// Flip a bit in data
			data[0] ^= 0x01
			if id.Verify(data, sig) {
				t.Error("Verification succeeded for modified data")
			}
			data[0] ^= 0x01 // flip back
		}

		if len(sig) > 0 {
			// Flip a bit in signature
			sig[0] ^= 0x01
			if id.Verify(data, sig) {
				t.Error("Verification succeeded for modified signature")
			}
		}
	})
}

func FuzzIdentityEncryptDecrypt(f *testing.F) {
	f.Add([]byte("secret message"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		id, err := New()
		if err != nil {
			t.Fatalf("Failed to create identity: %v", err)
		}

		// Test without ratchet
		ciphertext, err := id.Encrypt(plaintext, nil)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		decrypted, err := id.Decrypt(ciphertext, nil, false, nil)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Decrypted data mismatch: %x != %x", decrypted, plaintext)
		}

		// Test with ratchet
		ratchetPriv, err := id.RotateRatchet()
		if err != nil {
			t.Fatalf("RotateRatchet failed: %v", err)
		}

		// Derive public key from ratchet private key
		ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
		if err != nil {
			t.Fatalf("Failed to derive ratchet public key: %v", err)
		}

		ciphertext2, err := id.Encrypt(plaintext, ratchetPub)
		if err != nil {
			t.Fatalf("Encrypt with ratchet failed: %v", err)
		}

		decrypted2, err := id.Decrypt(ciphertext2, [][]byte{ratchetPriv}, true, nil)
		if err != nil {
			t.Fatalf("Decrypt with ratchet failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted2) {
			t.Errorf("Decrypted data with ratchet mismatch: %x != %x", decrypted2, plaintext)
		}
	})
}
