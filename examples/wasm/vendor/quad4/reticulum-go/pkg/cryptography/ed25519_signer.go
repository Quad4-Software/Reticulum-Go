// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
)

// Ed25519Signer signs Reticulum identity material with Ed25519. Use a software
// implementation from seed ([NewSoftwareEd25519Signer]) or wrap an HSM-backed
// [crypto.Signer] with [NewEd25519SignerFromCryptoSigner]. The public key must
// be the 32-byte Ed25519 key used in identity announcements.
type Ed25519Signer interface {
	Sign(message []byte) ([]byte, error)
	Ed25519PublicKey() ed25519.PublicKey
}

type softwareEd25519Signer struct {
	seed []byte
}

// NewSoftwareEd25519Signer returns a signer that uses an in-memory Ed25519 seed
// (same semantics as identity file bytes 32:64).
func NewSoftwareEd25519Signer(seed []byte) (Ed25519Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("ed25519 seed must be 32 bytes")
	}
	s := make([]byte, ed25519.SeedSize)
	copy(s, seed)
	return &softwareEd25519Signer{seed: s}, nil
}

func (s *softwareEd25519Signer) Sign(message []byte) ([]byte, error) {
	priv := ed25519.NewKeyFromSeed(s.seed)
	return Sign(priv, message), nil
}

func (s *softwareEd25519Signer) Ed25519PublicKey() ed25519.PublicKey {
	return ed25519.NewKeyFromSeed(s.seed).Public().(ed25519.PublicKey)
}

// cryptoSignerAdapter wraps crypto.Signer for Ed25519 (HSM / PKCS#11 etc.).
type cryptoSignerAdapter struct {
	s crypto.Signer
}

// NewEd25519SignerFromCryptoSigner wraps an Ed25519 crypto.Signer, such as
// PKCS#11 or a hardware key exposed via the standard library interface.
func NewEd25519SignerFromCryptoSigner(s crypto.Signer) (Ed25519Signer, error) {
	if s == nil {
		return nil, errors.New("nil crypto.Signer")
	}
	pub, ok := s.Public().(ed25519.PublicKey)
	if !ok || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("crypto.Signer public key must be Ed25519 (32 bytes)")
	}
	return &cryptoSignerAdapter{s: s}, nil
}

func (a *cryptoSignerAdapter) Sign(message []byte) ([]byte, error) {
	return a.s.Sign(rand.Reader, message, crypto.Hash(0))
}

func (a *cryptoSignerAdapter) Ed25519PublicKey() ed25519.PublicKey {
	return a.s.Public().(ed25519.PublicKey)
}
