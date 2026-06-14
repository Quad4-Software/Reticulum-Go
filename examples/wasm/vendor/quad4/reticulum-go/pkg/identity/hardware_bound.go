// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"

	"quad4/reticulum-go/pkg/cryptography"
)

const (
	hardwareBoundMagicLen   = 4
	hardwareBoundHeaderLen  = 8
	hardwareBoundV1FileSize = hardwareBoundHeaderLen + 32 + 32
)

var hardwareBoundMagic = []byte("RHB1")

// OptionalIdentitySignerHook supplies a signer when [LoadIdentityFile] loads a
// hardware-bound file and the signer argument is nil (e.g. set in init for PKCS#11).
var OptionalIdentitySignerHook func() (cryptography.Ed25519Signer, error)

// IsHardwareBoundIdentityFilePayload reports whether data is a v1 RHB1 descriptor.
func IsHardwareBoundIdentityFilePayload(data []byte) bool {
	if len(data) != hardwareBoundV1FileSize {
		return false
	}
	if !bytes.Equal(data[:hardwareBoundMagicLen], hardwareBoundMagic) {
		return false
	}
	return data[4] == 1
}

// WriteHardwareBoundIdentityFile writes RHB1 v1: 8-byte header, X25519 priv, Ed25519 pub (no seed).
func WriteHardwareBoundIdentityFile(path string, x25519Private []byte, ed25519Pub ed25519.PublicKey) error {
	if len(x25519Private) != 32 {
		return errors.New("x25519 private key must be 32 bytes")
	}
	if len(ed25519Pub) != ed25519.PublicKeySize {
		return errors.New("ed25519 public key must be 32 bytes")
	}
	buf := make([]byte, hardwareBoundV1FileSize)
	copy(buf[0:4], hardwareBoundMagic)
	buf[4] = 1
	copy(buf[8:40], x25519Private)
	copy(buf[40:72], ed25519Pub)
	// #nosec G304 G703 -- path is operator-chosen identity storage
	return os.WriteFile(path, buf, 0600)
}

// ToHardwareBoundFile writes RHB1 v1 (X25519 private + Ed25519 public on disk).
// Requires exportable key material; fails if signing is already external-only.
func (i *Identity) ToHardwareBoundFile(path string) error {
	if i.externalSigner != nil {
		return ErrSigningMaterialNotExportable
	}
	pk, err := i.GetPrivateKey()
	if err != nil {
		return err
	}
	return WriteHardwareBoundIdentityFile(path, pk[:32], i.verificationKey)
}

// LoadIdentityFile loads 64-byte software identity bytes or RHB1 v1 hardware-bound descriptor.
// Hardware-bound paths need a matching signer or [OptionalIdentitySignerHook].
func LoadIdentityFile(path string, signer cryptography.Ed25519Signer) (*Identity, error) {
	// #nosec G304 G703 -- path is operator-chosen identity storage
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == KeySize/8 {
		return FromBytes(data)
	}
	if !IsHardwareBoundIdentityFilePayload(data) {
		return nil, fmt.Errorf("identity file: want %d bytes (software) or %d bytes (hardware-bound v1), got %d", KeySize/8, hardwareBoundV1FileSize, len(data))
	}
	s := signer
	if s == nil && OptionalIdentitySignerHook != nil {
		s, err = OptionalIdentitySignerHook()
		if err != nil {
			return nil, fmt.Errorf("identity optional signer hook: %w", err)
		}
	}
	if s == nil {
		return nil, ErrHardwareBoundSignerRequired
	}
	return identityFromHardwareBoundV1(data, s)
}

func identityFromHardwareBoundV1(data []byte, signer cryptography.Ed25519Signer) (*Identity, error) {
	if data[4] != 1 {
		return nil, fmt.Errorf("unsupported hardware-bound identity version %d", data[4])
	}
	if data[5] != 0 || data[6] != 0 || data[7] != 0 {
		return nil, errors.New("hardware-bound identity: reserved header bytes must be zero")
	}
	x := append([]byte(nil), data[8:40]...)
	edPub := append(ed25519.PublicKey(nil), data[40:72]...)
	if len(edPub) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key length in descriptor")
	}
	if !bytes.Equal(signer.Ed25519PublicKey(), edPub) {
		return nil, ErrHardwareBoundSignerPublicKeyMismatch
	}
	return NewIdentityWithSigner(x, signer)
}
