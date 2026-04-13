package identity

import (
	"bytes"
	"path/filepath"
	"testing"

	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
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
	if len(h) != TRUNCATED_HASHLENGTH/8 {
		t.Errorf("Expected hash length %d, got %d", TRUNCATED_HASHLENGTH/8, len(h))
	}

	hexHash := id.Hex()
	if len(hexHash) != TRUNCATED_HASHLENGTH/4 {
		t.Errorf("Expected hex hash length %d, got %d", TRUNCATED_HASHLENGTH/4, len(hexHash))
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
	if len(ratchet) != RATCHETSIZE/8 {
		t.Errorf("Expected ratchet size %d, got %d", RATCHETSIZE/8, len(ratchet))
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
	if len(h) != TRUNCATED_HASHLENGTH/8 {
		t.Errorf("Expected length %d, got %d", TRUNCATED_HASHLENGTH/8, len(h))
	}
}

func TestGetRandomHash(t *testing.T) {
	h := GetRandomHash()
	if len(h) != TRUNCATED_HASHLENGTH/8 {
		t.Errorf("Expected length %d, got %d", TRUNCATED_HASHLENGTH/8, len(h))
	}
}
