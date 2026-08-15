// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"testing"
)

func TestTokenRoundTrip(t *testing.T) {
	plain := []byte("hello reticulum protocol")
	for _, size := range []int{TokenKeySize, TokenKeySize128} {
		key := make([]byte, size)
		for i := range key {
			key[i] = byte(i)
		}
		ct, err := EncryptToken(key, plain)
		if err != nil {
			t.Fatalf("size %d encrypt: %v", size, err)
		}
		if len(ct) < TokenOverhead {
			t.Fatalf("size %d token shorter than overhead: %d", size, len(ct))
		}
		got, err := DecryptToken(key, ct)
		if err != nil {
			t.Fatalf("size %d decrypt: %v", size, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("size %d got %q want %q", size, got, plain)
		}
	}
}

func TestTokenRejectsBadHMAC(t *testing.T) {
	key := make([]byte, TokenKeySize)
	ct, err := EncryptToken(key, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	ct[len(ct)-1] ^= 0xFF
	if _, err := DecryptToken(key, ct); err == nil {
		t.Fatal("expected HMAC failure")
	}
}

func TestTokenRejectsBadKeySize(t *testing.T) {
	if _, err := EncryptToken([]byte("short"), []byte("x")); err == nil {
		t.Fatal("expected key size error")
	}
	if _, err := DecryptToken([]byte("short"), make([]byte, TokenOverhead+16)); err == nil {
		t.Fatal("expected decrypt key size error")
	}
}
