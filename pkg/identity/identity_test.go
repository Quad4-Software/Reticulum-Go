// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
)

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
	err := id.ToFile(idPath)
	if err != nil {
		t.Fatalf("ToFile failed: %v", err)
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
