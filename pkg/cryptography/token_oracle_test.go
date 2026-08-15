// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"crypto/aes"
	"strings"
	"testing"
)

type pythonTokenVector struct {
	key, plaintext, token []byte
}

var pythonToken64Vectors = []pythonTokenVector{
	{bytesSeq(64), nil, mustHex("e53b00eaed8f64c4104107a02117abbd768b1995e8e640c6ff07495784b535ffe8e58931ea1351c654ebc6fd5b1e61e1f4fd8dfa700263abfcd4d3134ef52eeb")},
	{bytesSeq(64), []byte("A"), mustHex("dc304ecb704cbf6b2eeb0d3d65347c07a20f223e73c4496db1c7c8a01bc07b16f4e3ded351b2a0f2cb569a104dc5065c80758fb08fdaff5de18d233a98f04e8c")},
	{bytesSeq(64), []byte("hello reticulum protocol"), mustHex("afe7d46f6bce42b94ff3de9a2e47a859a3123695989cd5da5ec77668cc63a4fe0b1f7007c02cbbf4092903fa0fb492d82b8d4d61bf587513b9c632d47d66b36af81e1005cc656ff118fb2db5a30b8a3b")},
	{bytesSeq(64), bytes.Repeat([]byte{'x'}, 16), mustHex("3881d6b085d3eaff45fe542e45e3133aafc8b90bedff00b09046bb92e1e3a1d59ad5a78f92b195f5afd87b2936ed5b807da4774f0439fdc8348bdea341f6ec4c31643ac5088dfeeab999a9afefb8b470")},
	{bytesSeq(64), bytes.Repeat([]byte{'x'}, 17), mustHex("6f9790915f52a5e31e2eca45c129eeacf0efe465392b8057afaab914ab95f7fa11f5d7cb500ba1e1d06c8f0a23a5897f1953a9c6660ab259d3d3649a234c7f04d70dfdd0c7a263a56c34767b0f83cfa6")},
}

var pythonToken32Vectors = []pythonTokenVector{
	{bytesSeq(32), nil, mustHex("5b52d43697ad5cc2c725188c6de5e88984a74deb7ddef107e612747ed9b4e41307cec5a48fe468f9ac90ede5088595e7c905cc6851d766604bf8510e3dfe60db")},
	{bytesSeq(32), []byte("A"), mustHex("b9f218db410f0b460e22164f05155db80b185f41a70ca25342f55650f2d9b47dec3eecf1421260904161969357ff60e0372271eb750108f6d1011eae31602064")},
	{bytesSeq(32), []byte("hello reticulum protocol"), mustHex("0842887863eeb37c03d86de03079ff5c5d121d6971ea792f70cc4cac82251682f57cfe0eefb8f82cd1b32a01bced157664ecf7082c3cfaa687d253f1615ea6fd460fe17d2408ea571cea407d70a13c67")},
	{bytesSeq(32), bytes.Repeat([]byte{'x'}, 16), mustHex("972d848b409c97b3dfd5dad432a2bba30b33cf3ee82b54b52c92a1882f872884ea51c52218eb2fa14d1c93f78f61f085a9636f2ad2b69e9daf282479571cca6e1dc0e087c8a3f3d486b0b79befe751ac")},
	{bytesSeq(32), bytes.Repeat([]byte{'x'}, 17), mustHex("00181a60c8d150224dacf7822115b81d3d60311161bfd6e4c5d5b37b73b900397b70897216ec08a85c144752bb1ff7061db41e3d6722c2cd8dcfecbc408010d1c83432b0832602b65d5130dd3f175e18")},
}

// Guarantee: Token is IV || AES-CBC(PKCS7(pt)) || HMAC-SHA256(hmacKey, IV||ciphertext).
func TestOracleTokenLayoutAndHMAC(t *testing.T) {
	plain := []byte("BH_TOKEN_LAYOUT")
	for _, size := range []int{TokenKeySize, TokenKeySize128} {
		key := bytesSeq(size)
		tok, err := EncryptToken(key, plain)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if len(tok) < TokenOverhead+aes.BlockSize {
			t.Fatalf("size %d token too short: %d", size, len(tok))
		}
		mac := tok[len(tok)-SHA256Size:]
		body := tok[:len(tok)-SHA256Size]
		hmacKey, encKey, err := splitTokenKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if !ValidateHMAC(hmacKey, body, mac) {
			t.Fatalf("size %d HMAC over IV||ciphertext failed", size)
		}
		got, err := decryptAESCBC(encKey, body)
		if err != nil {
			t.Fatalf("size %d body decrypt: %v", size, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("size %d body decrypt mismatch", size)
		}
	}
	t.Log("PROVED TOKEN_LAYOUT_HMAC")
}

// Guarantee: tampering IV, ciphertext, or MAC fails HMAC and never returns plaintext.
func TestOracleTokenRejectsTamper(t *testing.T) {
	key := bytesSeq(TokenKeySize)
	plain := []byte("BH_TOKEN_TAMPER")
	tok, err := EncryptToken(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	offsets := []int{0, aes.BlockSize, len(tok) - 1}
	for _, off := range offsets {
		bad := append([]byte(nil), tok...)
		bad[off] ^= 0x01
		got, err := DecryptToken(key, bad)
		if err == nil && bytes.Equal(got, plain) {
			t.Fatalf("accepted tamper at offset %d", off)
		}
		if err == nil {
			t.Fatalf("tamper at %d returned err=nil plaintext=%q", off, got)
		}
		if !strings.Contains(err.Error(), "HMAC") {
			t.Fatalf("tamper at %d err=%v want HMAC failure", off, err)
		}
	}
	t.Log("PROVED TOKEN_HMAC_TAMPER")
}

// Guarantee: captured Python RNS Token blobs decrypt in Go for AES-256 and AES-128 keys.
func TestOracleTokenPythonVectors(t *testing.T) {
	for i, v := range pythonToken64Vectors {
		got, err := DecryptToken(v.key, v.token)
		if err != nil {
			t.Fatalf("token64[%d]: %v", i, err)
		}
		if !bytes.Equal(got, v.plaintext) {
			t.Fatalf("token64[%d] got %q want %q", i, got, v.plaintext)
		}
	}
	for i, v := range pythonToken32Vectors {
		got, err := DecryptToken(v.key, v.token)
		if err != nil {
			t.Fatalf("token32[%d]: %v", i, err)
		}
		if !bytes.Equal(got, v.plaintext) {
			t.Fatalf("token32[%d] got %q want %q", i, got, v.plaintext)
		}
	}
	t.Log("PROVED TOKEN_PYTHON_VECTORS")
}

func TestOracleTokenTooShort(t *testing.T) {
	key := bytesSeq(TokenKeySize)
	if _, err := DecryptToken(key, make([]byte, TokenOverhead-1)); err == nil {
		t.Fatal("expected short token error")
	}
	if _, err := DecryptToken(key, nil); err == nil {
		t.Fatal("expected nil token error")
	}
}

func TestOracleGenerateTokenKeySize(t *testing.T) {
	key, err := GenerateTokenKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != TokenKeySize {
		t.Fatalf("GenerateTokenKey len=%d want %d", len(key), TokenKeySize)
	}
}
