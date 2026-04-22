// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package resource

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func computeMapHash(chunk, randomHash []byte) []byte {
	h := sha256.New()
	h.Write(chunk)
	h.Write(randomHash)
	out := h.Sum(nil)
	return out[:MapHashLen]
}

func TestPrepareOutboundForLink_DoesNotCorruptCiphertext(t *testing.T) {
	const sdu = 64
	const payloadSize = 200

	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i & 0xFF)
	}

	res, err := New(payload, false)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	var capturedPlain []byte
	identityEncrypt := func(plain []byte) ([]byte, error) {
		capturedPlain = append([]byte(nil), plain...)
		marker := bytes.Repeat([]byte{0xAB, 0xCD, 0xEF, 0x12}, (len(plain)+3)/4)
		marker = marker[:len(plain)]
		return marker, nil
	}

	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	if capturedPlain == nil {
		t.Fatal("encrypt was not called")
	}

	cipher := res.outboundCipher
	if cipher == nil {
		t.Fatal("outboundCipher is nil")
	}

	expected := bytes.Repeat([]byte{0xAB, 0xCD, 0xEF, 0x12}, (len(cipher)+3)/4)
	expected = expected[:len(cipher)]
	if !bytes.Equal(cipher, expected) {
		for i := range cipher {
			if cipher[i] != expected[i] {
				t.Fatalf("ciphertext corrupted at byte %d: got 0x%02x, want 0x%02x (sdu=%d)", i, cipher[i], expected[i], sdu)
			}
		}
		t.Fatal("ciphertext mismatch")
	}
}

func TestPrepareOutboundForLink_HashmapMatchesPostHashCiphertext(t *testing.T) {
	const sdu = 50
	payload := bytes.Repeat([]byte{0x55}, 300)

	res, err := New(payload, false)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	identityEncrypt := func(plain []byte) ([]byte, error) {
		out := make([]byte, len(plain))
		for i := range plain {
			out[i] = byte((int(plain[i]) ^ i) & 0xFF)
		}
		return out, nil
	}

	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	parts := int(res.GetSegments())
	for i := 0; i < parts; i++ {
		slice := res.OutboundCiphertextSlice(i, sdu)
		if slice == nil {
			t.Fatalf("nil slice at part %d", i)
		}
		idx := res.PartIndexForMapHash(computeMapHash(slice, res.GetRandomHash()))
		if idx != i {
			t.Fatalf("part %d hash does not map back: got idx=%d", i, idx)
		}
	}
}
