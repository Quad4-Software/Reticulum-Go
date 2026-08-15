// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/cryptography"
)

type countingDecryptProvider struct {
	inner        cryptography.CryptoProvider
	decryptCalls int
}

func (p *countingDecryptProvider) GenerateKeyPair() (privateKey, publicKey []byte, err error) {
	return p.inner.GenerateKeyPair()
}

func (p *countingDecryptProvider) PublicKeyFromPrivate(privateKey []byte) ([]byte, error) {
	return p.inner.PublicKeyFromPrivate(privateKey)
}

func (p *countingDecryptProvider) DeriveSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	return p.inner.DeriveSharedSecret(privateKey, peerPublicKey)
}

func (p *countingDecryptProvider) GetBasepoint() []byte {
	return p.inner.GetBasepoint()
}

func (p *countingDecryptProvider) GenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return p.inner.GenerateSigningKeyPair()
}

func (p *countingDecryptProvider) Sign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return p.inner.Sign(privateKey, message)
}

func (p *countingDecryptProvider) Verify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return p.inner.Verify(publicKey, message, signature)
}

func (p *countingDecryptProvider) EncryptAES256CBC(key, plaintext []byte) ([]byte, error) {
	return p.inner.EncryptAES256CBC(key, plaintext)
}

func (p *countingDecryptProvider) DecryptAES256CBC(key, ciphertext []byte) ([]byte, error) {
	p.decryptCalls++
	return p.inner.DecryptAES256CBC(key, ciphertext)
}

func (p *countingDecryptProvider) ComputeHMAC(key, message []byte) []byte {
	return p.inner.ComputeHMAC(key, message)
}

func (p *countingDecryptProvider) ValidateHMAC(key, message, messageHMAC []byte) bool {
	return p.inner.ValidateHMAC(key, message, messageHMAC)
}

func (p *countingDecryptProvider) Hash(data []byte) []byte {
	return p.inner.Hash(data)
}

func (p *countingDecryptProvider) DeriveKey(secret, salt, info []byte, length int) ([]byte, error) {
	return p.inner.DeriveKey(secret, salt, info, length)
}

func (p *countingDecryptProvider) ExpandEncryptWithHMACKeyMaterial(key32 []byte) (hmacKey, aesKey []byte, err error) {
	return p.inner.ExpandEncryptWithHMACKeyMaterial(key32)
}

func (p *countingDecryptProvider) DeriveIdentityKeyMaterial(sharedSecret, salt, context []byte) ([]byte, error) {
	return p.inner.DeriveIdentityKeyMaterial(sharedSecret, salt, context)
}

// Guarantee: Identity.Decrypt rejects MAC-tampered Encrypt tokens and never calls AES decrypt.
func TestOracleEncryptDecryptRejectsTamperedMACBeforeAES(t *testing.T) {
	orig := cryptography.ActiveProvider()
	counter := &countingDecryptProvider{inner: orig}
	cryptography.SetProvider(counter)
	defer cryptography.SetProvider(orig)

	sender, err := New()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := New()
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("BH_ID_ENCRYPT_HMAC_ORACLE")
	token, err := sender.Encrypt(plain, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(token) < 32+32+32 {
		t.Fatalf("token too short: %d", len(token))
	}

	tampered := append([]byte(nil), token...)
	tampered[len(tampered)-1] ^= 0x01

	got, err := receiver.Decrypt(tampered, nil, false, nil)
	if counter.decryptCalls != 0 {
		t.Fatalf("DecryptAES256CBC calls=%d want 0 on bad MAC", counter.decryptCalls)
	}
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("Decrypt accepted MAC-tampered token")
	}
	if err == nil || !strings.Contains(err.Error(), "invalid HMAC") {
		t.Fatalf("Decrypt err=%v want invalid HMAC", err)
	}
}

func TestOracleEncryptDecryptRejectsTamperedCiphertextBeforeAES(t *testing.T) {
	orig := cryptography.ActiveProvider()
	counter := &countingDecryptProvider{inner: orig}
	cryptography.SetProvider(counter)
	defer cryptography.SetProvider(orig)

	sender, err := New()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := New()
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("BH_ID_ENCRYPT_CT_ORACLE")
	token, err := sender.Encrypt(plain, nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tampered := append([]byte(nil), token...)
	tampered[32] ^= 0x01

	got, err := receiver.Decrypt(tampered, nil, false, nil)
	if counter.decryptCalls != 0 {
		t.Fatalf("DecryptAES256CBC calls=%d want 0 on bad MAC", counter.decryptCalls)
	}
	if err == nil && bytes.Equal(got, plain) {
		t.Fatal("Decrypt accepted ciphertext-tampered token")
	}
	if err == nil || !strings.Contains(err.Error(), "invalid HMAC") {
		t.Fatalf("Decrypt err=%v want invalid HMAC", err)
	}
}

func TestOracleEncryptDecryptRejectsTruncatedToken(t *testing.T) {
	orig := cryptography.ActiveProvider()
	counter := &countingDecryptProvider{inner: orig}
	cryptography.SetProvider(counter)
	defer cryptography.SetProvider(orig)

	sender, err := New()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := New()
	if err != nil {
		t.Fatal(err)
	}

	token, err := sender.Encrypt([]byte("short"), nil)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	short := token[:len(token)-1]

	_, err = receiver.Decrypt(short, nil, false, nil)
	if err == nil {
		t.Fatal("Decrypt accepted truncated token")
	}
	if counter.decryptCalls != 0 {
		t.Fatalf("DecryptAES256CBC calls=%d want 0 on truncated token", counter.decryptCalls)
	}
}
