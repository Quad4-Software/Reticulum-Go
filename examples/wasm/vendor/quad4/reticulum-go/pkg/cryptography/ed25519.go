// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto/ed25519"
	"crypto/rand"
)

// Ed25519SignatureSize is the byte length of an Ed25519 signature.
const Ed25519SignatureSize = ed25519.SignatureSize

func implGenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func implSign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return ed25519.Sign(privateKey, message)
}

func implVerify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(publicKey, message, signature)
}

// GenerateSigningKeyPair creates a random Ed25519 key pair.
func GenerateSigningKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ActiveProvider().GenerateSigningKeyPair()
}

// Sign signs message with privateKey.
func Sign(privateKey ed25519.PrivateKey, message []byte) []byte {
	return ActiveProvider().Sign(privateKey, message)
}

// Verify reports whether signature is valid for message under publicKey.
func Verify(publicKey ed25519.PublicKey, message, signature []byte) bool {
	return ActiveProvider().Verify(publicKey, message, signature)
}
