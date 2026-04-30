// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package ifac

import (
	"encoding/hex"
	"errors"
	"fmt"

	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
)

// IFACFlag is the high bit of the first packet header byte that signals an
// outer Interface Access Code is present in the packet.
const IFACFlag byte = 0x80

// MinSize is the smallest accepted IFAC size in bytes.
const MinSize = 1

// DefaultSize is the default outer Interface Access Code size in bytes for
// UDP, TCP, AutoInterface, BackboneInterface and similar interfaces.
const DefaultSize = 16

// SaltHex is the fixed HKDF salt used to turn a netname/passphrase pair into
// a 64-byte interface key.
const SaltHex = "adf54d882c9a9b80771eb4995d702d4a3e733391b2a0f53f416d9f907e55cff8"

// keyLen is the HKDF output size in bytes (32-byte X25519 + 32-byte Ed25519 seed).
const keyLen = 64

// Salt returns a fresh copy of the IFAC salt bytes.
func Salt() []byte {
	salt, err := hex.DecodeString(SaltHex)
	if err != nil {
		panic(fmt.Errorf("ifac: invalid salt constant: %w", err))
	}
	return salt
}

// Identity carries a fully derived Interface Access Code keypair plus the raw
// HKDF key material used as the HKDF salt for masking.
type Identity struct {
	size     int
	key      []byte
	identity *identity.Identity
}

// Size returns the per-interface IFAC size in bytes.
func (i *Identity) Size() int { return i.size }

// Key returns the 64-byte HKDF output used as the masking salt.
func (i *Identity) Key() []byte { return append([]byte(nil), i.key...) }

// Sign returns the last `size` bytes of the Ed25519 signature of `raw`.
func (i *Identity) Sign(raw []byte) ([]byte, error) {
	sig, err := i.identity.Sign(raw)
	if err != nil {
		return nil, err
	}
	if len(sig) < i.size {
		return nil, fmt.Errorf("ifac: signature shorter (%d) than ifac size (%d)", len(sig), i.size)
	}
	return sig[len(sig)-i.size:], nil
}

// IdentityHash returns the network identity hash used to label the IFAC.
func (i *Identity) IdentityHash() []byte { return i.identity.Hash() }

// New builds an Interface Access Code identity from a network name and
// passphrase. Either may be empty, but at least one must be set, which matches
// Reticulum's behaviour where IFAC is only enabled when the user supplies
// a network_name and/or passphrase.
//
// The size argument is clamped to at least MinSize. A size of 0 is treated as
// DefaultSize.
func New(size int, netname, netkey string) (*Identity, error) {
	if netname == "" && netkey == "" {
		return nil, errors.New("ifac: at least one of network_name or passphrase must be set")
	}
	if size <= 0 {
		size = DefaultSize
	}
	if size < MinSize {
		size = MinSize
	}

	var origin []byte
	if netname != "" {
		h := cryptography.Hash([]byte(netname))
		origin = append(origin, h...)
	}
	if netkey != "" {
		h := cryptography.Hash([]byte(netkey))
		origin = append(origin, h...)
	}
	originHash := cryptography.Hash(origin)

	key, err := cryptography.DeriveKey(originHash, Salt(), nil, keyLen)
	if err != nil {
		return nil, fmt.Errorf("ifac: hkdf derive failed: %w", err)
	}

	id, err := identity.FromBytes(key)
	if err != nil {
		return nil, fmt.Errorf("ifac: identity load failed: %w", err)
	}

	return &Identity{size: size, key: key, identity: id}, nil
}

// FromKey builds an IFAC identity directly from raw 64-byte key material.
// Useful for tests and for accepting an externally provisioned key.
func FromKey(size int, key []byte) (*Identity, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("ifac: key must be %d bytes, got %d", keyLen, len(key))
	}
	if size <= 0 {
		size = DefaultSize
	}
	if size < MinSize {
		size = MinSize
	}
	id, err := identity.FromBytes(key)
	if err != nil {
		return nil, fmt.Errorf("ifac: identity load failed: %w", err)
	}
	cp := append([]byte(nil), key...)
	return &Identity{size: size, key: cp, identity: id}, nil
}

// Mask wraps a raw outbound packet with an Interface Access Code and applies
// the per-byte HKDF mask, matching Transport.transmit. The returned slice
// is a freshly allocated buffer.
//
// Layout: [masked_header_byte_0 (IFAC flag forced on)] [masked_header_byte_1]
// [unmasked ifac (size bytes)] [masked rest of packet starting at original
// byte 2]
//
// raw must include the standard two-byte Reticulum packet header.
func (i *Identity) Mask(raw []byte) ([]byte, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("ifac: packet too short (%d bytes) for masking", len(raw))
	}
	ifac, err := i.Sign(raw)
	if err != nil {
		return nil, err
	}

	maskLen := len(raw) + i.size
	mask, err := cryptography.DeriveKey(ifac, i.key, nil, maskLen)
	if err != nil {
		return nil, fmt.Errorf("ifac: hkdf mask derive failed: %w", err)
	}

	newHeader := []byte{raw[0] | IFACFlag, raw[1]}
	newRaw := make([]byte, 0, maskLen)
	newRaw = append(newRaw, newHeader...)
	newRaw = append(newRaw, ifac...)
	newRaw = append(newRaw, raw[2:]...)

	masked := make([]byte, len(newRaw))
	for k, b := range newRaw {
		switch {
		case k == 0:
			masked[k] = (b ^ mask[k]) | IFACFlag
		case k == 1 || k > i.size+1:
			masked[k] = b ^ mask[k]
		default:
			masked[k] = b
		}
	}
	return masked, nil
}

// Unmask reverses Mask and verifies the embedded Interface Access Code.
// Returns the recovered raw packet (without the IFAC) and a boolean that is
// true when verification succeeded. When the IFAC flag is not set, the input
// is returned unchanged with ok=true.
//
// The caller is responsible for separately enforcing the policy "if IFAC is
// configured for this interface but the IFAC flag is not set, drop the
// packet" -- this function only validates packets that claim to carry an
// IFAC.
func (i *Identity) Unmask(raw []byte) ([]byte, bool, error) {
	if len(raw) < 2 {
		return nil, false, fmt.Errorf("ifac: packet too short (%d bytes) for unmasking", len(raw))
	}
	if raw[0]&IFACFlag != IFACFlag {
		return raw, true, nil
	}
	if len(raw) <= 2+i.size {
		return nil, false, nil
	}

	mask, err := cryptography.DeriveKey(raw[2:2+i.size], i.key, nil, len(raw))
	if err != nil {
		return nil, false, fmt.Errorf("ifac: hkdf unmask derive failed: %w", err)
	}

	unmasked := make([]byte, len(raw))
	for k, b := range raw {
		if k <= 1 || k > i.size+1 {
			unmasked[k] = b ^ mask[k]
		} else {
			unmasked[k] = b
		}
	}

	header := []byte{unmasked[0] & 0x7f, unmasked[1]}
	rebuilt := make([]byte, 0, 2+(len(unmasked)-2-i.size))
	rebuilt = append(rebuilt, header...)
	rebuilt = append(rebuilt, unmasked[2+i.size:]...)

	expected, err := i.Sign(rebuilt)
	if err != nil {
		return nil, false, err
	}

	if !equal(expected, raw[2:2+i.size]) {
		return nil, false, nil
	}
	return rebuilt, true, nil
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
