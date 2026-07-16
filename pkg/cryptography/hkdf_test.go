// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"

	"quad4/pbt/pkg/pbt"
)

func TestDeriveKey(t *testing.T) {
	secret := []byte("test-secret")
	salt := []byte("test-salt")
	info := []byte("test-info")
	length := 32 // Desired key length

	key1, err := DeriveKey(secret, salt, info, length)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	if len(key1) != length {
		t.Errorf("DeriveKey returned key of length %d; want %d", len(key1), length)
	}

	// Derive another key with the same parameters, should be identical
	key2, err := DeriveKey(secret, salt, info, length)
	if err != nil {
		t.Fatalf("Second DeriveKey failed: %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Errorf("DeriveKey is not deterministic. Got %x and %x for the same inputs", key1, key2)
	}

	// Derive a key with different info, should be different
	differentInfo := []byte("different-info")
	key3, err := DeriveKey(secret, salt, differentInfo, length)
	if err != nil {
		t.Fatalf("DeriveKey with different info failed: %v", err)
	}
	if bytes.Equal(key1, key3) {
		t.Errorf("DeriveKey produced the same key for different info strings")
	}

	// Derive a key with different salt, should be different
	differentSalt := []byte("different-salt")
	key4, err := DeriveKey(secret, differentSalt, info, length)
	if err != nil {
		t.Fatalf("DeriveKey with different salt failed: %v", err)
	}
	if bytes.Equal(key1, key4) {
		t.Errorf("DeriveKey produced the same key for different salts")
	}

	// Derive a key with different secret, should be different
	differentSecret := []byte("different-secret")
	key5, err := DeriveKey(differentSecret, salt, info, length)
	if err != nil {
		t.Fatalf("DeriveKey with different secret failed: %v", err)
	}
	if bytes.Equal(key1, key5) {
		t.Errorf("DeriveKey produced the same key for different secrets")
	}

	// Derive a key with different length
	differentLength := 64
	key6, err := DeriveKey(secret, salt, info, differentLength)
	if err != nil {
		t.Fatalf("DeriveKey with different length failed: %v", err)
	}
	if len(key6) != differentLength {
		t.Errorf("DeriveKey returned key of length %d; want %d", len(key6), differentLength)
	}
}

func TestDeriveKeyEdgeCases(t *testing.T) {
	secret := []byte("test-secret")
	salt := []byte("test-salt")
	info := []byte("test-info")

	t.Run("EmptySecret", func(t *testing.T) {
		_, err := DeriveKey([]byte{}, salt, info, 32)
		if err == nil {
			t.Errorf("DeriveKey should fail with empty secret")
		}
	})

	t.Run("EmptySalt", func(t *testing.T) {
		_, err := DeriveKey(secret, []byte{}, info, 32)
		if err != nil {
			t.Errorf("DeriveKey failed with empty salt: %v", err)
		}
	})

	t.Run("EmptyInfo", func(t *testing.T) {
		_, err := DeriveKey(secret, salt, []byte{}, 32)
		if err != nil {
			t.Errorf("DeriveKey failed with empty info: %v", err)
		}
	})

	t.Run("ZeroLength", func(t *testing.T) {
		_, err := DeriveKey(secret, salt, info, 0)
		if err == nil {
			t.Errorf("DeriveKey should fail with zero length")
		}
	})
}

type hkdfInputs struct {
	secret, salt, info []byte
	length             int
}

func genHKDFInputs(r *rand.Rand, size int) hkdfInputs {
	secLen := 1 + r.Intn(64)
	saltLen := r.Intn(65)
	infoLen := r.Intn(129)
	maxOut := 64
	if size > 0 && size < maxOut {
		maxOut = size
	}
	if maxOut < 1 {
		maxOut = 1
	}
	outLen := 1 + r.Intn(maxOut)
	secret := make([]byte, secLen)
	salt := make([]byte, saltLen)
	info := make([]byte, infoLen)
	for i := range secret {
		secret[i] = byte(r.Intn(256))
	}
	for i := range salt {
		salt[i] = byte(r.Intn(256))
	}
	for i := range info {
		info[i] = byte(r.Intn(256))
	}
	return hkdfInputs{secret: secret, salt: salt, info: info, length: outLen}
}

func TestPBTHKDFDeterministic(t *testing.T) {
	gen := pbt.NewGenerator("hkdfInputs", genHKDFInputs)
	prop := pbt.ForAll(
		"derive key is deterministic",
		gen,
		func(in hkdfInputs) bool {
			k1, err := DeriveKey(in.secret, in.salt, in.info, in.length)
			if err != nil {
				panic(err)
			}
			k2, err := DeriveKey(in.secret, in.salt, in.info, in.length)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(k1, k2) && len(k1) == in.length
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(7), pbt.WithMaxSize(64))
}

func BenchmarkDeriveKey(b *testing.B) {
	secret := []byte("bench-secret-material")
	salt := []byte("bench-salt")
	info := []byte{}
	for _, length := range []int{64, 256, 1024} {
		b.Run("Len-"+strconv.Itoa(length), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(length))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out, err := DeriveKey(secret, salt, info, length)
				if err != nil {
					b.Fatal(err)
				}
				if len(out) != length {
					b.Fatalf("len=%d want %d", len(out), length)
				}
			}
		})
	}
}

// TestDeriveKeyAllocBudget keeps HKDF expand from allocating a new HMAC per
// output block (the pre-fix pattern scaled with length).
func TestDeriveKeyAllocBudget(t *testing.T) {
	secret := []byte("budget-secret")
	salt := []byte("budget-salt")
	allocs := testing.AllocsPerRun(200, func() {
		_, err := DeriveKey(secret, salt, nil, 1024)
		if err != nil {
			t.Fatal(err)
		}
	})
	// Extract HMAC + expand HMAC + PRK + output + a few small temps.
	if allocs > 25 {
		t.Fatalf("DeriveKey(1024) allocs=%.1f want <= 25", allocs)
	}
}
