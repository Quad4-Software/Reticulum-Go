// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package cryptography

import (
	"bytes"
	"math/rand"
	"testing"

	"quad4/pbt/pkg/pbt"
)

func byteSliceNonEmpty(maxLen int) pbt.Generator[[]byte] {
	return pbt.Map(
		"nonEmptyBytes",
		pbt.SliceOf(pbt.IntRange(0, 255), 1, maxLen),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
}

func byteSliceMaybeEmpty(maxLen int) pbt.Generator[[]byte] {
	return pbt.Map(
		"bytes",
		pbt.SliceOf(pbt.IntRange(0, 255), 0, maxLen),
		func(xs []int) []byte {
			b := make([]byte, len(xs))
			for i, v := range xs {
				b[i] = byte(v)
			}
			return b
		},
	)
}

type hkdfInputs struct {
	secret, salt, info []byte
	length             int
}

func genHKDFInputs(r *rand.Rand, size int) hkdfInputs {
	secLen := 1 + r.Intn(64)
	saltLen := r.Intn(65)
	infoLen := r.Intn(129)
	maxOut := 64
	if size > 0 && size < maxOut {
		maxOut = size
	}
	if maxOut < 1 {
		maxOut = 1
	}
	outLen := 1 + r.Intn(maxOut)
	secret := make([]byte, secLen)
	salt := make([]byte, saltLen)
	info := make([]byte, infoLen)
	for i := range secret {
		secret[i] = byte(r.Intn(256))
	}
	for i := range salt {
		salt[i] = byte(r.Intn(256))
	}
	for i := range info {
		info[i] = byte(r.Intn(256))
	}
	return hkdfInputs{secret: secret, salt: salt, info: info, length: outLen}
}

func TestPBTHKDFDeterministic(t *testing.T) {
	gen := pbt.NewGenerator("hkdfInputs", genHKDFInputs)
	prop := pbt.ForAll(
		"derive key is deterministic",
		gen,
		func(in hkdfInputs) bool {
			k1, err := DeriveKey(in.secret, in.salt, in.info, in.length)
			if err != nil {
				panic(err)
			}
			k2, err := DeriveKey(in.secret, in.salt, in.info, in.length)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(k1, k2) && len(k1) == in.length
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(7), pbt.WithMaxSize(64))
}

func TestPBTHMACValidate(t *testing.T) {
	keyMsg := pbt.Tuple2(
		"keyMsg",
		byteSliceNonEmpty(64),
		byteSliceMaybeEmpty(2048),
	)
	prop := pbt.ForAll(
		"hmac validate accepts computed mac and rejects tamper",
		keyMsg,
		func(in pbt.Tuple2Value[[]byte, []byte]) bool {
			key := in.First
			msg := in.Second
			mac := ComputeHMAC(key, msg)
			if !ValidateHMAC(key, msg, mac) {
				return false
			}
			if len(msg) > 0 {
				tam := bytes.Clone(msg)
				tam[0] ^= 0x01
				if ValidateHMAC(key, tam, mac) {
					return false
				}
			}
			if len(mac) > 0 {
				bad := bytes.Clone(mac)
				bad[0] ^= 0x01
				if ValidateHMAC(key, msg, bad) {
					return false
				}
			}
			return true
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(100), pbt.WithSeed(11))
}

func TestPBTAESCBCRoundTrip(t *testing.T) {
	ptGen := byteSliceMaybeEmpty(512)
	prop := pbt.ForAll(
		"aes-256-cbc encrypt decrypt",
		ptGen,
		func(plaintext []byte) bool {
			key, err := GenerateAES256Key()
			if err != nil {
				panic(err)
			}
			ct, err := EncryptAES256CBC(key, plaintext)
			if err != nil {
				panic(err)
			}
			out, err := DecryptAES256CBC(key, ct)
			if err != nil {
				panic(err)
			}
			return bytes.Equal(out, plaintext)
		},
		pbt.WithShrinker(pbt.SliceShrinker[byte]()),
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(99))
}
