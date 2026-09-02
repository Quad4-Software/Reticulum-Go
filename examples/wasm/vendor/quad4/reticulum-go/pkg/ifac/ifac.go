// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package ifac

import (
	"encoding/hex"
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/identity"
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

var saltBytes = mustDecodeHex(SaltHex)

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Errorf("ifac: invalid salt constant: %w", err))
	}
	return b
}

// Salt returns a fresh copy of the IFAC salt bytes.
func Salt() []byte {
	return append([]byte(nil), saltBytes...)
}

// Identity carries a fully derived Interface Access Code keypair plus the raw
// HKDF key material used as the HKDF salt for masking.
type Identity struct {
	size     int
	key      []byte
	identity *identity.Identity
	scratch  []byte
}

func (i *Identity) ensureScratch(n int) []byte {
	if cap(i.scratch) < n {
		i.scratch = make([]byte, n)
	}
	return i.scratch[:n]
}

// Size returns the per-interface IFAC size in bytes.
func (i *Identity) Size() int { return i.size }

// Key returns the 64-byte HKDF output used as the masking salt.
func (i *Identity) Key() []byte { return append([]byte(nil), i.key...) }

// Sign returns the last size bytes of the Ed25519 signature of raw.
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
	if netname != "" && netkey != "" {
		origin = make([]byte, 0, 64)
	}
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

// MaskInto wraps raw with an IFAC into dst when cap(dst) is large enough.
// When cap(dst) is too small a new buffer is allocated. Mask delegates here.
func (i *Identity) MaskInto(dst, raw []byte) ([]byte, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("ifac: packet too short (%d bytes) for masking", len(raw))
	}
	ifacSig, err := i.Sign(raw)
	if err != nil {
		return nil, err
	}

	maskLen := len(raw) + i.size
	if cap(dst) < maskLen {
		dst = make([]byte, maskLen)
	} else {
		dst = dst[:maskLen]
	}
	dst[0] = raw[0] | IFACFlag
	dst[1] = raw[1]
	copy(dst[2:], ifacSig)
	copy(dst[2+i.size:], raw[2:])

	mask := i.ensureScratch(maskLen)
	if err := cryptography.DeriveKeyInto(mask, ifacSig, i.key, nil); err != nil {
		return nil, fmt.Errorf("ifac: hkdf mask derive failed: %w", err)
	}
	for k, b := range dst {
		switch {
		case k == 0:
			dst[k] = (b ^ mask[k]) | IFACFlag
		case k == 1 || k > i.size+1:
			dst[k] = b ^ mask[k]
		}
	}
	return dst, nil
}

// Mask wraps a raw outbound packet with an Interface Access Code and applies
// the per-byte HKDF mask, matching Transport.transmit.
func (i *Identity) Mask(raw []byte) ([]byte, error) {
	return i.MaskInto(nil, raw)
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

	mask := i.ensureScratch(len(raw))
	if err := cryptography.DeriveKeyInto(mask, raw[2:2+i.size], i.key, nil); err != nil {
		return nil, false, fmt.Errorf("ifac: hkdf unmask derive failed: %w", err)
	}

	rebuilt := make([]byte, 2+len(raw)-2-i.size)
	rebuilt[0] = (raw[0] ^ mask[0]) & 0x7f
	rebuilt[1] = raw[1] ^ mask[1]
	for src := 2 + i.size; src < len(raw); src++ {
		dst := src - i.size
		rebuilt[dst] = raw[src] ^ mask[src]
	}

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
