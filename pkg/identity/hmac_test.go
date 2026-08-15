// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/cryptography"
)

func TestHMACKeyComputeValidate(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	key := id.GenerateHMACKey()
	if len(key) != KeySize/8 {
		t.Fatalf("HMAC key len %d", len(key))
	}
	msg := []byte("reticulum-hmac")
	mac := id.ComputeHMAC(key, msg)
	if !id.ValidateHMAC(key, msg, mac) {
		t.Fatal("ValidateHMAC failed for fresh mac")
	}
	mac[0] ^= 0xff
	if id.ValidateHMAC(key, msg, mac) {
		t.Fatal("tampered mac must fail")
	}
}

func TestEncryptDecryptWithHMACRoundTrip(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, keyLen := range []int{32, 64} {
		key := make([]byte, keyLen)
		for i := range key {
			key[i] = byte(i + 1)
		}
		plain := []byte("payload-hmac")
		ct, err := id.EncryptWithHMAC(plain, key)
		if err != nil {
			t.Fatalf("EncryptWithHMAC(%d): %v", keyLen, err)
		}
		got, err := id.DecryptWithHMAC(ct, key)
		if err != nil {
			t.Fatalf("DecryptWithHMAC(%d): %v", keyLen, err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("round trip mismatch keyLen=%d", keyLen)
		}
	}
	if _, err := id.EncryptWithHMAC([]byte("x"), []byte("short")); err == nil {
		t.Fatal("invalid key length must fail")
	}
	if _, err := id.DecryptWithHMAC([]byte("short"), make([]byte, 32)); err == nil {
		t.Fatal("short ciphertext must fail")
	}
}

func TestPBTEncryptDecryptWithHMAC(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	pt := pbt.Map(
		"[]byte",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 1024),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"EncryptWithHMAC round trip",
		pt,
		func(plain []byte) bool {
			key := make([]byte, 32)
			for i := range key {
				key[i] = byte(i * 3)
			}
			ct, err := id.EncryptWithHMAC(plain, key)
			if err != nil {
				return false
			}
			got, err := id.DecryptWithHMAC(ct, key)
			if err != nil {
				return false
			}
			if !bytes.Equal(got, plain) {
				return false
			}
			if len(ct) >= cryptography.SHA256Size {
				ct[len(ct)-1] ^= 0x01
				_, err := id.DecryptWithHMAC(ct, key)
				return err != nil
			}
			return true
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(9))
}

func TestStringAndHex(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != id.Hex() || id.Hex() == "" {
		t.Fatalf("String/Hex mismatch %q %q", id.String(), id.Hex())
	}
}

// FuzzDecryptWithHMAC ensures arbitrary ciphertext never panics.
func FuzzDecryptWithHMAC(f *testing.F) {
	id, err := New()
	if err != nil {
		f.Fatal(err)
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	if ct, err := id.EncryptWithHMAC([]byte("seed"), key); err == nil {
		f.Add(ct)
	}
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x02})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<14 {
			t.Skip()
		}
		_, _ = id.DecryptWithHMAC(data, key)
	})
}
