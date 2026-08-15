// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"sync"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/securemem"
)

// NewIdentityWithSigner builds an identity whose Ed25519 operations go through
// signer (e.g. HSM via [cryptography.NewEd25519SignerFromCryptoSigner]). The
// X25519 half is still supplied as a 32-byte private scalar (often generated
// in software or from a second key in the HSM). On-wire format is unchanged.
func NewIdentityWithSigner(x25519Private []byte, signer cryptography.Ed25519Signer) (*Identity, error) {
	if len(x25519Private) != 32 {
		return nil, errors.New("invalid X25519 private key length")
	}
	if signer == nil {
		return nil, errors.New("nil Ed25519 signer")
	}
	pub, err := cryptography.PublicKeyFromPrivate(x25519Private)
	if err != nil {
		return nil, err
	}
	vk := signer.Ed25519PublicKey()
	if len(vk) != ed25519.PublicKeySize {
		return nil, errors.New("Ed25519 public key from signer must be 32 bytes")
	}

	i := &Identity{
		publicKey:       pub,
		signingSeed:     nil,
		verificationKey: append(ed25519.PublicKey(nil), vk...),
		externalSigner:  signer,
		ratchets:        make(map[string]*securemem.Buf),
		ratchetExpiry:   make(map[string]int64),
		mutex:           &sync.RWMutex{},
	}
	if err := storeX25519(i, x25519Private); err != nil {
		return nil, err
	}

	combinedPub := make([]byte, KeySize/8)
	copy(combinedPub[:KeySize/16], i.publicKey)
	copy(combinedPub[KeySize/16:], i.verificationKey)
	fullHash := cryptography.Hash(combinedPub)
	i.hash = fullHash[:TruncatedHashLength/8]
	i.hexHash = hex.EncodeToString(i.hash)

	return i, nil
}
