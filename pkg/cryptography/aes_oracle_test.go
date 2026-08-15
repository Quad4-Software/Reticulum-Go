// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"testing"
)

// Python RNS AES_256_CBC.encrypt(PKCS7.pad(pt), key=bytes(range(32)), iv=bytes(range(16,32)))
// with IV prepended, matching EncryptAES256CBC wire layout.
var pythonAES256CBCVector = struct {
	key, iv, plaintext, blob []byte
}{
	key:       bytesSeq(32),
	iv:        bytesSeqFrom(16, 16),
	plaintext: []byte("hello reticulum protocol"),
	blob:      mustHex("101112131415161718191a1b1c1d1e1f3b4fbf63498dafc12ec4598b19653dca10c121ad51674aabef9ed09f17904962"),
}

func bytesSeq(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func bytesSeqFrom(start, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(start + i)
	}
	return b
}

func mustHex(s string) []byte {
	out, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return out
}

// Guarantee: AES-256-CBC ciphertext is IV || PKCS7-CBC and decrypts with stdlib.
func TestOracleAES256CBCLayoutMatchesStdlib(t *testing.T) {
	key := bytesSeq(32)
	plaintexts := [][]byte{
		nil,
		{},
		[]byte("A"),
		[]byte("This is 16 bytes"),
		[]byte("hello reticulum protocol"),
		bytes.Repeat([]byte{'x'}, 32),
		bytes.Repeat([]byte{'x'}, 33),
	}
	for _, pt := range plaintexts {
		ct, err := EncryptAES256CBC(key, pt)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if len(ct) < aes.BlockSize*2 {
			t.Fatalf("ciphertext too short: %d", len(ct))
		}
		if (len(ct)-aes.BlockSize)%aes.BlockSize != 0 {
			t.Fatalf("body not block aligned: %d", len(ct))
		}
		iv := ct[:aes.BlockSize]
		body := ct[aes.BlockSize:]
		block := newAESBlock(t, key)
		padded := stdlibCBCDecrypt(block, iv, body)
		got, err := RemovePKCS7Padding(padded)
		if err != nil {
			t.Fatalf("unpad: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatalf("stdlib unpad mismatch got %q want %q", got, pt)
		}

		wantBody := make([]byte, len(body))
		copy(wantBody, addPKCS7Padding(pt))
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(wantBody, wantBody)
		if !bytes.Equal(body, wantBody) {
			t.Fatal("EncryptCBC body mismatch versus stdlib")
		}
	}
	t.Log("PROVED AES256_CBC_LAYOUT")
}

// Guarantee: empty plaintext encrypts to IV plus one full padding block.
func TestOracleAES256CBCEmptyPlaintext(t *testing.T) {
	key := bytesSeq(32)
	ct, err := EncryptAES256CBC(key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) != aes.BlockSize*2 {
		t.Fatalf("empty plaintext ct len=%d want 32", len(ct))
	}
	got, err := DecryptAES256CBC(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty round-trip got %q", got)
	}
	t.Log("PROVED AES256_EMPTY_PADDING_BLOCK")
}

// Guarantee: a captured Python RNS AES-256-CBC blob decrypts in Go.
func TestOracleAES256CBCPythonVector(t *testing.T) {
	v := pythonAES256CBCVector
	if !bytes.Equal(v.blob[:aes.BlockSize], v.iv) {
		t.Fatal("fixture IV prefix mismatch")
	}
	got, err := DecryptAES256CBC(v.key, v.blob)
	if err != nil {
		t.Fatalf("DecryptAES256CBC: %v", err)
	}
	if !bytes.Equal(got, v.plaintext) {
		t.Fatalf("got %q want %q", got, v.plaintext)
	}
	t.Log("PROVED AES256_PYTHON_VECTOR")
}
