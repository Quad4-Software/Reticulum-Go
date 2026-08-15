// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/securemem"
)

func newSecBuf(n int) (*securemem.Buf, error) {
	return securemem.New(n)
}

func storeX25519(i *Identity, priv []byte) error {
	if len(priv) != 32 {
		return errors.New("invalid X25519 private key length")
	}
	if i.privateKey != nil {
		_ = i.privateKey.Close()
		i.privateKey = nil
	}
	buf, err := newSecBuf(32)
	if err != nil {
		return err
	}
	if err := buf.CopyFrom(priv); err != nil {
		_ = buf.Close()
		return err
	}
	i.privateKey = buf
	return nil
}

func storeSigningMaterial(i *Identity, seed []byte) error {
	if len(seed) != ed25519.SeedSize {
		return errors.New("invalid Ed25519 seed length")
	}
	if i.signingSeed != nil {
		_ = i.signingSeed.Close()
		i.signingSeed = nil
	}
	if i.signingKey != nil {
		_ = i.signingKey.Close()
		i.signingKey = nil
	}
	seedBuf, err := newSecBuf(ed25519.SeedSize)
	if err != nil {
		return err
	}
	if err := seedBuf.CopyFrom(seed); err != nil {
		_ = seedBuf.Close()
		return err
	}
	expanded := ed25519.NewKeyFromSeed(seed)
	keyBuf, err := newSecBuf(ed25519.PrivateKeySize)
	if err != nil {
		_ = seedBuf.Close()
		return err
	}
	if err := keyBuf.CopyFrom(expanded); err != nil {
		_ = seedBuf.Close()
		_ = keyBuf.Close()
		securemem.WipeBytes(expanded)
		return err
	}
	securemem.WipeBytes(expanded)
	i.signingSeed = seedBuf
	i.signingKey = keyBuf
	i.verificationKey = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(i.verificationKey, keyBuf.Bytes()[32:])
	return nil
}

func clearSigningMaterial(i *Identity) {
	if i.signingSeed != nil {
		_ = i.signingSeed.Close()
		i.signingSeed = nil
	}
	if i.signingKey != nil {
		_ = i.signingKey.Close()
		i.signingKey = nil
	}
}

func (i *Identity) hasExportablePrivate() bool {
	return i != nil &&
		i.externalSigner == nil &&
		i.privateKey != nil && i.privateKey.Len() == 32 &&
		i.signingSeed != nil && i.signingSeed.Len() == ed25519.SeedSize
}

func (i *Identity) hasDecryptPrivate() bool {
	return i != nil && i.privateKey != nil && i.privateKey.Len() == 32
}

// Close wipes locked private key material. Safe to call multiple times.
func (i *Identity) Close() {
	if i == nil {
		return
	}
	if i.privateKey != nil {
		_ = i.privateKey.Close()
		i.privateKey = nil
	}
	clearSigningMaterial(i)
	if i.mutex != nil {
		i.mutex.Lock()
		for id, buf := range i.ratchets {
			if buf != nil {
				_ = buf.Close()
			}
			delete(i.ratchets, id)
			delete(i.ratchetExpiry, id)
		}
		i.mutex.Unlock()
	}
}

// Wipe is an alias for Close.
func (i *Identity) Wipe() {
	i.Close()
}

func loadPrivateInto(i *Identity, privateKey, signingSeed []byte) error {
	if len(privateKey) != 32 || len(signingSeed) != 32 {
		return errors.New("invalid private key length")
	}
	if err := storeX25519(i, privateKey); err != nil {
		return err
	}
	pub, err := cryptography.PublicKeyFromPrivate(i.privateKey.Bytes())
	if err != nil {
		i.Close()
		return fmt.Errorf("failed to derive X25519 public key: %w", err)
	}
	i.publicKey = pub
	if err := storeSigningMaterial(i, signingSeed); err != nil {
		i.Close()
		return err
	}
	return nil
}
