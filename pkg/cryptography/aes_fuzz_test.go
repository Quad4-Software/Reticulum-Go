// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"testing"
)

func FuzzAES256CBCRoundTrip(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add(bytes.Repeat([]byte{0xA5}, 16))
	f.Add(bytes.Repeat([]byte{0xA5}, 17))

	key := bytesSeq(32)
	f.Fuzz(func(t *testing.T, pt []byte) {
		if len(pt) > 1<<12 {
			pt = pt[:1<<12]
		}
		ct, err := EncryptAES256CBC(key, pt)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		got, err := DecryptAES256CBC(key, ct)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatal("round-trip mismatch")
		}
	})
}

func FuzzDecryptAES256CBCNoPanic(f *testing.F) {
	f.Add(make([]byte, 32), make([]byte, 48))
	f.Add(make([]byte, 16), make([]byte, 16))
	f.Add([]byte{1}, []byte{2})

	f.Fuzz(func(t *testing.T, key, ct []byte) {
		if len(ct) > 1<<12 {
			ct = ct[:1<<12]
		}
		_, _ = DecryptAES256CBC(key, ct)
		if len(key) == AES256KeySize && len(ct) >= aes.BlockSize && (len(ct)-aes.BlockSize)%aes.BlockSize == 0 {
			if _, err := DecryptAES256CBC(key, ct); err == nil {
				body := ct[aes.BlockSize:]
				if len(body) == 0 {
					t.Fatal("accepted IV-only ciphertext")
				}
			}
		}
	})
}
