package cryptography

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"errors"
	"io"
	"testing"
)

type nonEd25519Signer struct{}

func (nonEd25519Signer) Public() crypto.PublicKey {
	return []byte{1, 2, 3}
}

func (nonEd25519Signer) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) {
	return nil, errors.New("unused")
}

func TestGenerateSigningKeyPair(t *testing.T) {
	pub1, priv1, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatalf("GenerateSigningKeyPair failed: %v", err)
	}

	if len(pub1) != ed25519.PublicKeySize {
		t.Errorf("Public key length is %d, want %d", len(pub1), ed25519.PublicKeySize)
	}
	if len(priv1) != ed25519.PrivateKeySize {
		t.Errorf("Private key length is %d, want %d", len(priv1), ed25519.PrivateKeySize)
	}

	// Generate another pair, should be different
	pub2, priv2, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatalf("Second GenerateSigningKeyPair failed: %v", err)
	}
	if pub1.Equal(pub2) {
		t.Error("Generated public keys are identical")
	}
	if priv1.Equal(priv2) {
		t.Error("Generated private keys are identical")
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, priv, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatalf("GenerateSigningKeyPair failed: %v", err)
	}

	message := []byte("This message needs to be signed.")

	signature := Sign(priv, message)
	if len(signature) != ed25519.SignatureSize {
		t.Errorf("Signature length is %d, want %d", len(signature), ed25519.SignatureSize)
	}

	// Verify correct signature
	if !Verify(pub, message, signature) {
		t.Errorf("Verify failed for a valid signature")
	}

	// Verify with tampered message
	tamperedMessage := append(message, '!')
	if Verify(pub, tamperedMessage, signature) {
		t.Errorf("Verify succeeded for a tampered message")
	}

	// Verify with tampered signature
	tamperedSignature := append(signature[:len(signature)-1], ^signature[len(signature)-1])
	if Verify(pub, message, tamperedSignature) {
		t.Errorf("Verify succeeded for a tampered signature")
	}

	// Verify with wrong public key
	wrongPub, _, _ := GenerateSigningKeyPair()
	if Verify(wrongPub, message, signature) {
		t.Errorf("Verify succeeded with the wrong public key")
	}

	// Verify empty message
	emptyMessage := []byte("")
	emptySig := Sign(priv, emptyMessage)
	if !Verify(pub, emptyMessage, emptySig) {
		t.Errorf("Verify failed for an empty message")
	}
	if Verify(pub, message, emptySig) {
		t.Errorf("Verify succeeded comparing non-empty message with empty signature")
	}
}

func TestSoftwareEd25519Signer(t *testing.T) {
	_, priv, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	seed := priv.Seed()
	s, err := NewSoftwareEd25519Signer(seed)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("hello")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if !Verify(pub, msg, sig) {
		t.Error("software signer signature did not verify")
	}
	if !bytes.Equal(s.Ed25519PublicKey(), pub) {
		t.Error("Ed25519PublicKey mismatch")
	}
}

func TestEd25519SignerFromCryptoSigner(t *testing.T) {
	_, priv, err := GenerateSigningKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewEd25519SignerFromCryptoSigner(priv)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("pkcs11-style")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if !Verify(pub, msg, sig) {
		t.Error("crypto.Signer wrapper did not verify")
	}
	_, err = NewEd25519SignerFromCryptoSigner(nil)
	if err == nil {
		t.Fatal("expected error for nil Signer")
	}
	_, err = NewEd25519SignerFromCryptoSigner(nonEd25519Signer{})
	if err == nil {
		t.Fatal("expected error for non-Ed25519 public key")
	}
}
