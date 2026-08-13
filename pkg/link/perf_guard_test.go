// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/rand"
	"testing"
)

func primedSessionLink(t testing.TB) *Link {
	t.Helper()
	l := &Link{mode: ModeDefault}
	session := make([]byte, 32)
	hmac := make([]byte, 32)
	if _, err := rand.Read(session); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(hmac); err != nil {
		t.Fatal(err)
	}
	if err := setSecBuf(&l.sessionKey, session); err != nil {
		t.Fatal(err)
	}
	if err := setSecBuf(&l.hmacKey, hmac); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.closeAllSecretKeys() })
	return l
}

func BenchmarkLinkEncryptDecrypt(b *testing.B) {
	l := primedSessionLink(b)
	payload := make([]byte, 64)
	if _, err := rand.Read(payload); err != nil {
		b.Fatal(err)
	}
	ct, err := l.encrypt(payload)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := l.encrypt(payload)
		if err != nil {
			b.Fatal(err)
		}
		_, err = l.decrypt(enc)
		if err != nil {
			b.Fatal(err)
		}
		_ = ct
	}
}

func BenchmarkLinkEncrypt(b *testing.B) {
	l := primedSessionLink(b)
	payload := make([]byte, 64)
	if _, err := rand.Read(payload); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := l.encrypt(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLinkEncryptAllocBudget(t *testing.T) {
	l := primedSessionLink(t)
	payload := make([]byte, 64)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	// Warm crypto.
	if _, err := l.encrypt(payload); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(200, func() {
		if _, err := l.encrypt(payload); err != nil {
			t.Fatal(err)
		}
	})
	// copySessionKeys (2) plus AES/HMAC ciphertext and intermediates.
	if allocs > 12 {
		t.Fatalf("Link.encrypt allocs=%.1f want <= 12", allocs)
	}
}

func TestLinkEncryptDecryptAllocBudget(t *testing.T) {
	l := primedSessionLink(t)
	payload := make([]byte, 64)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	ct, err := l.encrypt(payload)
	if err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(200, func() {
		enc, err := l.encrypt(payload)
		if err != nil {
			t.Fatal(err)
		}
		pt, err := l.decrypt(enc)
		if err != nil {
			t.Fatal(err)
		}
		_ = pt
		_ = ct
	})
	if allocs > 40 {
		t.Fatalf("Link encrypt+decrypt allocs=%.1f want <= 40", allocs)
	}
}
