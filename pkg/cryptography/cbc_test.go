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

func TestEncryptDecryptCBCMatchesStdlib(t *testing.T) {
	key := make([]byte, AES256KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}

	plaintexts := [][]byte{
		bytes.Repeat([]byte{0x11}, aes.BlockSize),
		bytes.Repeat([]byte{0x22}, aes.BlockSize*2),
		bytes.Repeat([]byte{0x33}, aes.BlockSize*5),
		make([]byte, aes.BlockSize),
	}
	for _, pt := range plaintexts {
		iv := make([]byte, aes.BlockSize)
		if _, err := rand.Read(iv); err != nil {
			t.Fatal(err)
		}

		want := make([]byte, len(pt))
		copy(want, pt)
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(want, want)

		got := append([]byte(nil), pt...)
		if err := EncryptCBC(block, iv, got); err != nil {
			t.Fatalf("EncryptCBC: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("encrypt mismatch\ngot  %x\nwant %x", got, want)
		}

		dec := make([]byte, len(got))
		if err := DecryptCBC(block, iv, got, dec); err != nil {
			t.Fatalf("DecryptCBC: %v", err)
		}
		if !bytes.Equal(dec, pt) {
			t.Fatalf("decrypt mismatch\ngot  %x\nwant %x", dec, pt)
		}

		inPlace := append([]byte(nil), got...)
		if err := DecryptCBC(block, iv, inPlace, inPlace); err != nil {
			t.Fatalf("DecryptCBC in place: %v", err)
		}
		if !bytes.Equal(inPlace, pt) {
			t.Fatalf("in-place decrypt mismatch")
		}
	}
}

func TestEncryptCBCRejectsBadArgs(t *testing.T) {
	key := make([]byte, AES256KeySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := EncryptCBC(nil, make([]byte, 16), make([]byte, 16)); err == nil {
		t.Fatal("expected nil block error")
	}
	if err := EncryptCBC(block, make([]byte, 15), make([]byte, 16)); err == nil {
		t.Fatal("expected bad iv error")
	}
	if err := EncryptCBC(block, make([]byte, 16), make([]byte, 15)); err == nil {
		t.Fatal("expected bad buf error")
	}
	if err := EncryptCBC(block, nil, make([]byte, 16)); err == nil {
		t.Fatal("expected nil iv error")
	}
}

func TestDecryptCBCRejectsBadArgs(t *testing.T) {
	key := make([]byte, AES256KeySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	if err := DecryptCBC(nil, make([]byte, 16), buf, buf); err == nil {
		t.Fatal("expected nil block error")
	}
	if err := DecryptCBC(block, make([]byte, 15), buf, buf); err == nil {
		t.Fatal("expected bad iv error")
	}
	if err := DecryptCBC(block, make([]byte, 16), buf, make([]byte, 32)); err == nil {
		t.Fatal("expected length mismatch error")
	}
	if err := DecryptCBC(block, make([]byte, 16), make([]byte, 15), make([]byte, 15)); err == nil {
		t.Fatal("expected unaligned error")
	}
}

func TestEncryptCBCEmptyBuffer(t *testing.T) {
	key := make([]byte, AES256KeySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, aes.BlockSize)
	if err := EncryptCBC(block, iv, nil); err != nil {
		t.Fatalf("empty encrypt: %v", err)
	}
	if err := DecryptCBC(block, iv, nil, nil); err != nil {
		t.Fatalf("empty decrypt: %v", err)
	}
}

func TestEncryptCBCDoesNotMutateIV(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, AES256KeySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	iv := bytes.Repeat([]byte{0xA5}, aes.BlockSize)
	wantIV := append([]byte(nil), iv...)
	buf := bytes.Repeat([]byte{0x3C}, aes.BlockSize*3)
	if err := EncryptCBC(block, iv, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(iv, wantIV) {
		t.Fatal("EncryptCBC mutated IV")
	}
	if err := DecryptCBC(block, iv, buf, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(iv, wantIV) {
		t.Fatal("DecryptCBC mutated IV")
	}
}

type oversizedBlock struct{}

func (oversizedBlock) BlockSize() int          { return 33 }
func (oversizedBlock) Encrypt(dst, src []byte) { copy(dst, src) }
func (oversizedBlock) Decrypt(dst, src []byte) { copy(dst, src) }

func TestDecryptCBCRejectsOversizedBlock(t *testing.T) {
	buf := make([]byte, 33)
	if err := DecryptCBC(oversizedBlock{}, buf, buf, buf); err == nil {
		t.Fatal("expected oversized block error")
	}
}
