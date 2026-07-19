// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"testing"

	"quad4/pbt/pkg/pbt"
)

// Property suite for cryptography. AES/HKDF/HMAC properties also live in
// aes_test.go, hkdf_test.go, and hmac_test.go.

func TestPBTX25519SharedSecretSymmetric(t *testing.T) {
	unit := pbt.Map("unit", pbt.IntRange(0, 0), func(int) struct{} { return struct{}{} })
	prop := pbt.ForAll(
		"x25519 shared secrets match",
		unit,
		func(struct{}) bool {
			aPriv, aPub, err := GenerateKeyPair()
			if err != nil {
				panic(err)
			}
			bPriv, bPub, err := GenerateKeyPair()
			if err != nil {
				panic(err)
			}
			ab, err := DeriveSharedSecret(aPriv, bPub)
			if err != nil {
				panic(err)
			}
			ba, err := DeriveSharedSecret(bPriv, aPub)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(ab, ba) && len(ab) == 32
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(40), pbt.WithSeed(21))
}
