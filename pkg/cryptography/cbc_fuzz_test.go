// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func FuzzEncryptDecryptCBC(f *testing.F) {
	f.Add(make([]byte, 16), make([]byte, 16), make([]byte, 16))
	f.Add(make([]byte, 32), make([]byte, 16), make([]byte, 32))
	f.Add([]byte{1, 2, 3}, make([]byte, 16), make([]byte, 16))
	f.Add(make([]byte, 0), make([]byte, 16), make([]byte, 0))

	f.Fuzz(func(t *testing.T, key, iv, buf []byte) {
		if len(key) != 16 && len(key) != 32 {
			return
		}
		if len(buf) > 1<<12 {
			buf = buf[:1<<12]
		}
		key = append([]byte(nil), key...)
		iv = append([]byte(nil), iv...)
		buf = append([]byte(nil), buf...)
		block, err := aes.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		orig := append([]byte(nil), buf...)
		ivEnc := append([]byte(nil), iv...)
		encErr := EncryptCBC(block, ivEnc, buf)
		if len(iv) != aes.BlockSize || len(orig)%aes.BlockSize != 0 {
			if encErr == nil {
				t.Fatal("EncryptCBC accepted invalid args")
			}
			return
		}
		if encErr != nil {
			t.Fatalf("EncryptCBC: %v", encErr)
		}
		want := append([]byte(nil), orig...)
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(want, want)
		if !bytes.Equal(buf, want) {
			t.Fatalf("encrypt mismatch versus stdlib\ngot  %x\nwant %x", buf, want)
		}

		plain := make([]byte, len(buf))
		if err := DecryptCBC(block, iv, buf, plain); err != nil {
			t.Fatalf("DecryptCBC: %v", err)
		}
		if !bytes.Equal(plain, orig) {
			t.Fatal("round-trip mismatch")
		}

		inPlace := append([]byte(nil), buf...)
		if err := DecryptCBC(block, iv, inPlace, inPlace); err != nil {
			t.Fatalf("DecryptCBC in place: %v", err)
		}
		if !bytes.Equal(inPlace, orig) {
			t.Fatal("in-place round-trip mismatch")
		}
	})
}
