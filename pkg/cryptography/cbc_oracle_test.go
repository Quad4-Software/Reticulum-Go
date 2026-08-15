// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"testing"
)

func stdlibCBCEncrypt(block cipher.Block, iv, pt []byte) []byte {
	out := append([]byte(nil), pt...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, out)
	return out
}

func stdlibCBCDecrypt(block cipher.Block, iv, ct []byte) []byte {
	out := append([]byte(nil), ct...)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, out)
	return out
}

func newAESBlock(t *testing.T, key []byte) cipher.Block {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	return block
}

// Guarantee: EncryptCBC/DecryptCBC match crypto/cipher CBC for AES-128 and AES-256.
func TestOracleCBCMatchesStdlib(t *testing.T) {
	keys := [][]byte{
		bytes.Repeat([]byte{0x01}, 16),
		bytes.Repeat([]byte{0x02}, 32),
	}
	lengths := []int{0, aes.BlockSize, aes.BlockSize * 2, aes.BlockSize * 7, 16 * 16}
	for _, key := range keys {
		block := newAESBlock(t, key)
		for _, n := range lengths {
			pt := make([]byte, n)
			if _, err := rand.Read(pt); err != nil {
				t.Fatal(err)
			}
			iv := make([]byte, aes.BlockSize)
			if _, err := rand.Read(iv); err != nil {
				t.Fatal(err)
			}

			want := stdlibCBCEncrypt(block, iv, pt)
			got := append([]byte(nil), pt...)
			if err := EncryptCBC(block, iv, got); err != nil {
				t.Fatalf("EncryptCBC n=%d: %v", n, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("encrypt n=%d mismatch", n)
			}

			plain := make([]byte, len(got))
			if err := DecryptCBC(block, iv, got, plain); err != nil {
				t.Fatalf("DecryptCBC n=%d: %v", n, err)
			}
			if !bytes.Equal(plain, pt) {
				t.Fatalf("decrypt n=%d mismatch", n)
			}
			if !bytes.Equal(stdlibCBCDecrypt(block, iv, got), pt) {
				t.Fatalf("stdlib decrypt n=%d mismatch", n)
			}
		}
	}
	t.Log("PROVED CBC_STDLIB_MATCH")
}

// Guarantee: in-place DecryptCBC equals decrypt-into-a-copy.
func TestOracleCBCDecryptInPlace(t *testing.T) {
	block := newAESBlock(t, bytes.Repeat([]byte{0x77}, 32))
	iv := bytes.Repeat([]byte{0x12}, aes.BlockSize)
	pt := bytes.Repeat([]byte{0xAB}, aes.BlockSize*4)
	ct := stdlibCBCEncrypt(block, iv, pt)

	copied := make([]byte, len(ct))
	if err := DecryptCBC(block, iv, ct, copied); err != nil {
		t.Fatal(err)
	}
	inPlace := append([]byte(nil), ct...)
	if err := DecryptCBC(block, iv, inPlace, inPlace); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, pt) || !bytes.Equal(inPlace, pt) {
		t.Fatal("in-place decrypt diverged from copy decrypt")
	}
	t.Log("PROVED CBC_INPLACE_DECRYPT")
}

// Guarantee: CBC chaining XORs with the previous ciphertext block, not plaintext.
func TestOracleCBCChainingUsesCiphertext(t *testing.T) {
	block := newAESBlock(t, bytes.Repeat([]byte{0x09}, 32))
	iv := bytes.Repeat([]byte{0x00}, aes.BlockSize)
	pt := make([]byte, aes.BlockSize*2)
	copy(pt[:aes.BlockSize], bytes.Repeat([]byte{0x11}, aes.BlockSize))
	copy(pt[aes.BlockSize:], bytes.Repeat([]byte{0x22}, aes.BlockSize))

	ct := append([]byte(nil), pt...)
	if err := EncryptCBC(block, iv, ct); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ct[:aes.BlockSize], ct[aes.BlockSize:]) {
		t.Fatal("two distinct plaintext blocks produced identical ciphertext")
	}
	if !bytes.Equal(ct, stdlibCBCEncrypt(block, iv, pt)) {
		t.Fatal("chaining mismatch versus stdlib")
	}
	t.Log("PROVED CBC_CHAINING")
}

// Guarantee: IV and body as adjacent subslices of one buffer match stdlib CBC.
func TestOracleCBCAdjacentIVAndBody(t *testing.T) {
	block := newAESBlock(t, bytes.Repeat([]byte{0x42}, 32))
	pt := bytes.Repeat([]byte{0x5A}, aes.BlockSize*3)
	out := make([]byte, aes.BlockSize+len(pt))
	if _, err := rand.Read(out[:aes.BlockSize]); err != nil {
		t.Fatal(err)
	}
	copy(out[aes.BlockSize:], pt)
	iv := append([]byte(nil), out[:aes.BlockSize]...)
	if err := EncryptCBC(block, out[:aes.BlockSize], out[aes.BlockSize:]); err != nil {
		t.Fatal(err)
	}
	want := stdlibCBCEncrypt(block, iv, pt)
	if !bytes.Equal(out[aes.BlockSize:], want) {
		t.Fatal("adjacent IV/body encrypt mismatch versus stdlib")
	}
	plain := make([]byte, len(pt))
	if err := DecryptCBC(block, out[:aes.BlockSize], out[aes.BlockSize:], plain); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, pt) {
		t.Fatal("adjacent IV/body decrypt mismatch")
	}
	t.Log("PROVED CBC_ADJACENT_IV_BODY")
}
