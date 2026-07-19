// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsutil

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
)

// Encoding selects how key material is printed or parsed.
type Encoding int

const (
	// EncHex is lowercase hex.
	EncHex Encoding = iota
	// EncBase64 is standard base64.
	EncBase64
	// EncBase32 is standard base32 without padding.
	EncBase32
)

// LoadIdentity loads a 64-byte identity file (.rid compatible).
func LoadIdentity(path string) (*identity.Identity, error) {
	return identity.FromFile(path)
}

// GenerateIdentity creates and optionally saves a new identity.
func GenerateIdentity(path string) (*identity.Identity, error) {
	id, err := identity.NewIdentity()
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := id.ToFile(path); err != nil {
			return nil, err
		}
	}
	return id, nil
}

// EncodeBytes encodes b using enc.
func EncodeBytes(b []byte, enc Encoding) string {
	switch enc {
	case EncBase64:
		return base64.StdEncoding.EncodeToString(b)
	case EncBase32:
		return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")
	default:
		return hex.EncodeToString(b)
	}
}

// DecodeBytes decodes s using enc.
func DecodeBytes(s string, enc Encoding) ([]byte, error) {
	s = strings.TrimSpace(s)
	switch enc {
	case EncBase64:
		return base64.StdEncoding.DecodeString(s)
	case EncBase32:
		pad := (8 - len(s)%8) % 8
		return base32.StdEncoding.DecodeString(s + strings.Repeat("=", pad))
	default:
		return hex.DecodeString(s)
	}
}

// ImportPublicIdentity builds an identity from a 64-byte public key encoding.
func ImportPublicIdentity(s string, enc Encoding) (*identity.Identity, error) {
	b, err := DecodeBytes(s, enc)
	if err != nil {
		return nil, err
	}
	if len(b) != 64 {
		return nil, fmt.Errorf("public key must be 64 bytes, got %d", len(b))
	}
	id := identity.FromPublicKey(b)
	if id == nil {
		return nil, fmt.Errorf("invalid public key")
	}
	return id, nil
}

// ImportPrivateIdentity builds an identity from a 64-byte private key encoding.
func ImportPrivateIdentity(s string, enc Encoding) (*identity.Identity, error) {
	b, err := DecodeBytes(s, enc)
	if err != nil {
		return nil, err
	}
	return identity.FromBytes(b)
}

// DestinationHashHex returns the hex destination hash for aspects on id.
func DestinationHashHex(id *identity.Identity, fullName string) (string, error) {
	app, aspects, err := destination.ParseName(fullName)
	if err != nil {
		return "", err
	}
	h := destination.Hash(id, app, aspects...)
	return hex.EncodeToString(h), nil
}

// SignFile signs the contents of path and returns the raw signature.
func SignFile(id *identity.Identity, path string) ([]byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied local path
	if err != nil {
		return nil, err
	}
	return id.Sign(data)
}

// VerifyFile verifies signature against the contents of path.
func VerifyFile(id *identity.Identity, path string, signature []byte) (bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied local path
	if err != nil {
		return false, err
	}
	return id.Verify(data, signature), nil
}

// WriteFileAtomic writes data to path with 0600 permissions.
func WriteFileAtomic(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600) // #nosec G306 -- identity/signature material
}
