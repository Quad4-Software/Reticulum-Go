// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package cryptography

import (
	"bytes"
	"testing"
)

func FuzzTokenRoundTrip(f *testing.F) {
	f.Add(byte(0), []byte("hello"))
	f.Add(byte(1), []byte{})
	f.Add(byte(0), bytes.Repeat([]byte{0x7f}, 64))

	f.Fuzz(func(t *testing.T, mode byte, pt []byte) {
		if len(pt) > 1<<12 {
			pt = pt[:1<<12]
		}
		size := TokenKeySize
		if mode&1 == 1 {
			size = TokenKeySize128
		}
		key := bytesSeq(size)
		tok, err := EncryptToken(key, pt)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		got, err := DecryptToken(key, tok)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if !bytes.Equal(got, pt) {
			t.Fatal("round-trip mismatch")
		}
	})
}

func FuzzDecryptTokenNoPanic(f *testing.F) {
	f.Add(make([]byte, 64), make([]byte, 64))
	f.Add(make([]byte, 32), make([]byte, 48))
	f.Add([]byte{1}, []byte{2})

	f.Fuzz(func(t *testing.T, key, tok []byte) {
		if len(tok) > 1<<12 {
			tok = tok[:1<<12]
		}
		_, _ = DecryptToken(key, tok)
	})
}
