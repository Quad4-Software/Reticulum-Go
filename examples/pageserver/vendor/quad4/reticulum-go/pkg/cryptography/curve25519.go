// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"crypto/rand"

	"golang.org/x/crypto/curve25519"
)

func implGenerateKeyPair() (privateKey, publicKey []byte, err error) {
	privateKey = make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, nil, err
	}

	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, err
	}

	return privateKey, publicKey, nil
}

func implDeriveSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	return curve25519.X25519(privateKey, peerPublicKey)
}

func implPublicKeyFromPrivate(privateKey []byte) ([]byte, error) {
	return curve25519.X25519(privateKey, curve25519.Basepoint)
}

// GenerateKeyPair creates a random X25519 key pair.
func GenerateKeyPair() (privateKey, publicKey []byte, err error) {
	return ActiveProvider().GenerateKeyPair()
}

// PublicKeyFromPrivate derives the X25519 public key from a private key.
func PublicKeyFromPrivate(privateKey []byte) ([]byte, error) {
	return ActiveProvider().PublicKeyFromPrivate(privateKey)
}

// DeriveSharedSecret performs an X25519 scalar multiplication.
func DeriveSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	return ActiveProvider().DeriveSharedSecret(privateKey, peerPublicKey)
}
