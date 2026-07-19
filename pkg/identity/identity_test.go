// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
)

func TestIdentityCloseWipesPrivate(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	priv, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != 64 {
		t.Fatalf("len %d", len(priv))
	}
	id.Close()
	if id.privateKey != nil || id.signingSeed != nil || id.signingKey != nil {
		t.Fatal("private buffers still set after Close")
	}
	_, err = id.GetPrivateKey()
	if err == nil {
		t.Fatal("GetPrivateKey should fail after Close")
	}
}

func TestNewIdentity(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if id == nil {
		t.Fatal("New() returned nil")
	}

	pubKey := id.GetPublicKey()
	if len(pubKey) != 64 {
		t.Errorf("Expected public key length 64, got %d", len(pubKey))
	}

	privKey, err := id.GetPrivateKey()
	if err != nil {
		t.Fatalf("GetPrivateKey: %v", err)
	}
	if len(privKey) != 64 {
		t.Errorf("Expected private key length 64, got %d", len(privKey))
	}
}

func TestNewIdentityWithSignerMatchesSoftware(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	pk, err := id.GetPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := cryptography.NewSoftwareEd25519Signer(pk[32:64])
	if err != nil {
		t.Fatal(err)
	}
	id2, err := NewIdentityWithSigner(pk[:32], signer)
	if err != nil {
		t.Fatal(err)
	}
	if id.GetHexHash() != id2.GetHexHash() {
		t.Errorf("hash mismatch: %s vs %s", id.GetHexHash(), id2.GetHexHash())
	}
	if !bytes.Equal(id.GetPublicKey(), id2.GetPublicKey()) {
		t.Error("public key mismatch")
	}
	msg := []byte("probe")
	sig1, err := id.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	sig2, err := id2.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sig1, sig2) {
		t.Error("signatures differ for same keys")
	}
	_, err = id2.GetPrivateKey()
	if err != ErrSigningMaterialNotExportable {
		t.Fatalf("GetPrivateKey: want %v, got %v", ErrSigningMaterialNotExportable, err)
	}
}

func TestSignVerify(t *testing.T) {
	id, _ := New()
	data := []byte("test data")
	sig, err := id.Sign(data)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if !id.Verify(data, sig) {
		t.Error("Verification failed for valid signature")
	}

	if id.Verify([]byte("wrong data"), sig) {
		t.Error("Verification succeeded for wrong data")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	id, _ := New()
	plaintext := []byte("secret message")

	ciphertext, err := id.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := id.Decrypt(ciphertext, nil, false, nil)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted data doesn't match plaintext: %q vs %q", decrypted, plaintext)
	}
}

func TestIdentityHash(t *testing.T) {
	id, _ := New()
	h := id.Hash()
	if len(h) != TruncatedHashLength/8 {
		t.Errorf("Expected hash length %d, got %d", TruncatedHashLength/8, len(h))
	}

	hexHash := id.Hex()
	if len(hexHash) != TruncatedHashLength/4 {
		t.Errorf("Expected hex hash length %d, got %d", TruncatedHashLength/4, len(hexHash))
	}
}

func TestFileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "identity")

	id, _ := New()
	if err := id.ToFile(idPath); err != nil {
		t.Fatalf("ToFile failed: %v", err)
	}

	st, err := os.Stat(idPath)
	if err != nil {
		t.Fatalf("stat identity: %v", err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("identity file mode %o allows group/other access", st.Mode().Perm())
	}

	loadedID, err := FromFile(idPath)
	if err != nil {
		t.Fatalf("FromFile failed: %v", err)
	}

	if !bytes.Equal(id.GetPublicKey(), loadedID.GetPublicKey()) {
		t.Error("Loaded identity public key doesn't match original")
	}
}

func TestRatchets(t *testing.T) {
	id, _ := New()

	ratchet, err := id.RotateRatchet()
	if err != nil {
		t.Fatalf("RotateRatchet failed: %v", err)
	}
	if len(ratchet) != RatchetSize/8 {
		t.Errorf("Expected ratchet size %d, got %d", RatchetSize/8, len(ratchet))
	}

	ratchets := id.GetRatchets()
	if len(ratchets) != 1 {
		t.Errorf("Expected 1 ratchet, got %d", len(ratchets))
	}

	id.CleanupExpiredRatchets()
	// Should still be there since it's not expired
	if len(id.GetRatchets()) != 1 {
		t.Error("Ratchet unexpectedly cleaned up")
	}
}

func TestRecallIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "identity_recall")

	id, _ := New()
	_ = id.ToFile(idPath)

	recalledID, err := RecallIdentity(idPath)
	if err != nil {
		t.Fatalf("RecallIdentity failed: %v", err)
	}

	if !bytes.Equal(id.GetPublicKey(), recalledID.GetPublicKey()) {
		t.Error("Recalled identity public key doesn't match original")
	}
}

func TestRecallMissingHashUsesErrIdentityNotFound(t *testing.T) {
	knownDestinationsLock.Lock()
	prev := knownDestinations
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prev
		knownDestinationsLock.Unlock()
	})

	hash := make([]byte, TruncatedHashLength/8)
	for i := range hash {
		hash[i] = byte(0xa0 + i)
	}
	_, err := Recall(hash)
	if !errors.Is(err, common.ErrIdentityNotFound) {
		t.Fatalf("got %v, want ErrIdentityNotFound", err)
	}
}

func TestTruncatedHash(t *testing.T) {
	data := []byte("some data")
	h := TruncatedHash(data)
	if len(h) != TruncatedHashLength/8 {
		t.Errorf("Expected length %d, got %d", TruncatedHashLength/8, len(h))
	}
}

func TestGetRandomHash(t *testing.T) {
	h := GetRandomHash()
	if len(h) != TruncatedHashLength/8 {
		t.Errorf("Expected length %d, got %d", TruncatedHashLength/8, len(h))
	}
}

func TestRememberStoresDefensiveCopies(t *testing.T) {
	knownDestinationsLock.Lock()
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()

	destBacking := make([]byte, 0, 64)
	destHash := append(destBacking, bytes.Repeat([]byte{0x11}, TruncatedHashLength/8)...)
	packet := append([]byte{}, []byte("packet-data")...)
	appData := append([]byte{}, []byte("app-data")...)
	pub := make([]byte, KeySize/8)
	key := hex.EncodeToString(destHash)

	Remember(packet, destHash, pub, appData)

	packet[0] ^= 0xFF
	destHash[0] ^= 0xFF
	appData[0] ^= 0xFF

	stored, ok := GetKnownDestination(key)
	if !ok {
		t.Fatal("expected stored destination")
	}

	gotPacket, ok := stored[0].([]byte)
	if !ok {
		t.Fatal("stored packet has unexpected type")
	}
	gotDest, ok := stored[1].([]byte)
	if !ok {
		t.Fatal("stored destination hash has unexpected type")
	}
	gotApp, ok := stored[3].([]byte)
	if !ok {
		t.Fatal("stored app data has unexpected type")
	}

	if bytes.Equal(gotPacket, packet) {
		t.Fatal("stored packet aliased caller buffer")
	}
	if bytes.Equal(gotDest, destHash) {
		t.Fatal("stored destination hash aliased caller buffer")
	}
	if bytes.Equal(gotApp, appData) {
		t.Fatal("stored app data aliased caller buffer")
	}
}

func TestGetKnownDestinationReturnsDefensiveCopies(t *testing.T) {
	knownDestinationsLock.Lock()
	knownDestinations = make(map[string][]any)
	knownDestinationsLock.Unlock()

	destHash := bytes.Repeat([]byte{0x22}, TruncatedHashLength/8)
	packet := []byte("packet-copy")
	appData := []byte("app-copy")
	pub := make([]byte, KeySize/8)
	Remember(packet, destHash, pub, appData)
	key := hex.EncodeToString(destHash)

	first, ok := GetKnownDestination(key)
	if !ok {
		t.Fatal("expected stored destination")
	}
	firstPacket := first[0].([]byte)
	firstPacket[0] ^= 0xFF

	second, ok := GetKnownDestination(key)
	if !ok {
		t.Fatal("expected stored destination on second fetch")
	}
	secondPacket := second[0].([]byte)
	if bytes.Equal(firstPacket, secondPacket) {
		t.Fatal("GetKnownDestination returned aliased packet slice")
	}
}

func TestRatchetKeyDefensiveCopies(t *testing.T) {
	ratchetPersistLock.Lock()
	knownRatchets = make(map[string][]byte)
	ratchetPersistLock.Unlock()

	id, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	input := []byte("ratchet-material")
	id.SetRatchetKey("peer", input)
	input[0] ^= 0xFF

	got, ok := id.GetRatchetKey("peer")
	if !ok {
		t.Fatal("expected ratchet key")
	}
	if bytes.Equal(got, input) {
		t.Fatal("stored ratchet key aliased caller input")
	}

	got[0] ^= 0xFF
	again, ok := id.GetRatchetKey("peer")
	if !ok {
		t.Fatal("expected ratchet key on second read")
	}
	if bytes.Equal(got, again) {
		t.Fatal("GetRatchetKey returned aliased internal ratchet key")
	}
}

func TestKnownRatchetsCap(t *testing.T) {
	ratchetPersistLock.Lock()
	knownRatchets = make(map[string][]byte)
	ratchetPersistLock.Unlock()
	t.Cleanup(func() {
		ratchetPersistLock.Lock()
		knownRatchets = make(map[string][]byte)
		ratchetPersistLock.Unlock()
	})

	id, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	for i := range MaxKnownRatchets + 32 {
		key := fmt.Sprintf("peer-%d", i)
		id.SetRatchetKey(key, []byte{byte(i), byte(i >> 8)})
	}

	ratchetPersistLock.Lock()
	n := len(knownRatchets)
	_, kept := knownRatchets[fmt.Sprintf("peer-%d", MaxKnownRatchets+31)]
	ratchetPersistLock.Unlock()
	if n > MaxKnownRatchets {
		t.Fatalf("knownRatchets size = %d, want <= %d", n, MaxKnownRatchets)
	}
	if !kept {
		t.Fatal("most recently inserted ratchet must not be evicted")
	}
}

func TestPBTIdentitySignVerify(t *testing.T) {
	msg := pbt.Map(
		"[]byte",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 4096),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"ed25519 sign and verify",
		msg,
		func(data []byte) bool {
			id, err := New()
			if err != nil {
				panic(err)
			}
			sig, err := id.Sign(data)
			if err != nil {
				panic(err)
			}
			if !id.Verify(data, sig) {
				return false
			}
			if len(data) > 0 {
				data[0] ^= 0x01
				if id.Verify(data, sig) {
					return false
				}
				data[0] ^= 0x01
			}
			if len(sig) > 0 {
				sig[0] ^= 0x01
				if id.Verify(data, sig) {
					return false
				}
			}
			return true
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(42))
}

func TestPBTIdentityEncryptDecrypt(t *testing.T) {
	pt := pbt.Map(
		"plaintext",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 4096),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"encrypt without ratchet then decrypt",
		pt,
		func(plaintext []byte) bool {
			id, err := New()
			if err != nil {
				panic(err)
			}
			ciphertext, err := id.Encrypt(plaintext, nil)
			if err != nil {
				panic(err)
			}
			decrypted, err := id.Decrypt(ciphertext, nil, false, nil)
			if err != nil {
				panic(err)
			}
			if !bytes.Equal(plaintext, decrypted) {
				return false
			}
			ratchetPriv, err := id.RotateRatchet()
			if err != nil {
				panic(err)
			}
			ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
			if err != nil {
				panic(err)
			}
			ciphertext2, err := id.Encrypt(plaintext, ratchetPub)
			if err != nil {
				panic(err)
			}
			decrypted2, err := id.Decrypt(ciphertext2, [][]byte{ratchetPriv}, true, nil)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(plaintext, decrypted2)
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(3))
}

func FuzzIdentitySignVerify(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		id, err := New()
		if err != nil {
			t.Fatalf("Failed to create identity: %v", err)
		}

		sig, err := id.Sign(data)
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		if !id.Verify(data, sig) {
			t.Error("Verification failed for valid signature")
		}

		if len(data) > 0 {
			// Flip a bit in data
			data[0] ^= 0x01
			if id.Verify(data, sig) {
				t.Error("Verification succeeded for modified data")
			}
			data[0] ^= 0x01 // flip back
		}

		if len(sig) > 0 {
			// Flip a bit in signature
			sig[0] ^= 0x01
			if id.Verify(data, sig) {
				t.Error("Verification succeeded for modified signature")
			}
		}
	})
}

func FuzzIdentityEncryptDecrypt(f *testing.F) {
	f.Add([]byte("secret message"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		id, err := New()
		if err != nil {
			t.Fatalf("Failed to create identity: %v", err)
		}

		// Test without ratchet
		ciphertext, err := id.Encrypt(plaintext, nil)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		decrypted, err := id.Decrypt(ciphertext, nil, false, nil)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Decrypted data mismatch: %x != %x", decrypted, plaintext)
		}

		// Test with ratchet
		ratchetPriv, err := id.RotateRatchet()
		if err != nil {
			t.Fatalf("RotateRatchet failed: %v", err)
		}

		// Derive public key from ratchet private key
		ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
		if err != nil {
			t.Fatalf("Failed to derive ratchet public key: %v", err)
		}

		ciphertext2, err := id.Encrypt(plaintext, ratchetPub)
		if err != nil {
			t.Fatalf("Encrypt with ratchet failed: %v", err)
		}

		decrypted2, err := id.Decrypt(ciphertext2, [][]byte{ratchetPriv}, true, nil)
		if err != nil {
			t.Fatalf("Decrypt with ratchet failed: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted2) {
			t.Errorf("Decrypted data with ratchet mismatch: %x != %x", decrypted2, plaintext)
		}
	})
}

func BenchmarkKnownDestinationsScale(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size-%d", size), func(b *testing.B) {
			// Clear map for each run
			knownDestinationsLock.Lock()
			knownDestinations = make(map[string][]any)
			knownDestinationsLock.Unlock()

			// Fill cache
			for range size {
				h := make([]byte, 16)
				_, _ = rand.Read(h)
				Remember([]byte("packet"), h, make([]byte, 64), []byte("appdata"))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				h := make([]byte, 16)
				// We use a small subset of the size for lookups to test hit performance
				for j := range 16 {
					h[j] = byte((i % size) >> (j * 8))
				}
				_, _ = Recall(h)
			}
		})
	}
}

func TestIdentityMemoryScale(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping identity memory test")
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	size := 100000
	t.Logf("Filling knownDestinations with %d entries...", size)

	for i := range size {
		h := make([]byte, 16)
		for j := range 16 {
			h[j] = byte(i >> (j * 8))
		}
		Remember([]byte("p"), h, make([]byte, 64), []byte("a"))
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	usedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory used for %d destinations: %.2f MB", size, usedMB)

	perEntry := (m2.Alloc - m1.Alloc) / uint64(size)
	t.Logf("Average per destination: %d bytes", perEntry)
}
