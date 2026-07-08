// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
	"quad4/reticulum-go/pkg/cryptography"
)

func TestPBTIdentitySignVerify(t *testing.T) {
	msg := pbt.Map(
		"[]byte",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 4096),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"ed25519 sign and verify",
		msg,
		func(data []byte) bool {
			id, err := New()
			if err != nil {
				panic(err)
			}
			sig, err := id.Sign(data)
			if err != nil {
				panic(err)
			}
			if !id.Verify(data, sig) {
				return false
			}
			if len(data) > 0 {
				data[0] ^= 0x01
				if id.Verify(data, sig) {
					return false
				}
				data[0] ^= 0x01
			}
			if len(sig) > 0 {
				sig[0] ^= 0x01
				if id.Verify(data, sig) {
					return false
				}
			}
			return true
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(42))
}

func TestPBTIdentityEncryptDecrypt(t *testing.T) {
	pt := pbt.Map(
		"plaintext",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, 4096),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
	prop := pbt.ForAll(
		"encrypt without ratchet then decrypt",
		pt,
		func(plaintext []byte) bool {
			id, err := New()
			if err != nil {
				panic(err)
			}
			ciphertext, err := id.Encrypt(plaintext, nil)
			if err != nil {
				panic(err)
			}
			decrypted, err := id.Decrypt(ciphertext, nil, false, nil)
			if err != nil {
				panic(err)
			}
			if !bytes.Equal(plaintext, decrypted) {
				return false
			}
			ratchetPriv, err := id.RotateRatchet()
			if err != nil {
				panic(err)
			}
			ratchetPub, err := cryptography.PublicKeyFromPrivate(ratchetPriv)
			if err != nil {
				panic(err)
			}
			ciphertext2, err := id.Encrypt(plaintext, ratchetPub)
			if err != nil {
				panic(err)
			}
			decrypted2, err := id.Decrypt(ciphertext2, [][]byte{ratchetPriv}, true, nil)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(plaintext, decrypted2)
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(60), pbt.WithSeed(3))
}
