// SPDX-License-Identifier: Apache-2.0
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
	for i := range parts {
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

func TestPartIndicesForMapHash_ReturnsAllDuplicateParts(t *testing.T) {
	const sdu = 32
	payload := bytes.Repeat([]byte{0x42}, 320)

	res, err := New(payload, false)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	identityEncrypt := func(plain []byte) ([]byte, error) {
		return bytes.Repeat([]byte{0xA5}, len(plain)), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	parts := int(res.GetSegments())
	if parts < 2 {
		t.Fatalf("expected multiple parts, got %d", parts)
	}

	firstSlice := res.OutboundCiphertextSlice(0, sdu)
	if len(firstSlice) == 0 {
		t.Fatal("first slice is empty")
	}
	targetHash := computeMapHash(firstSlice, res.GetRandomHash())

	indexes := res.PartIndicesForMapHash(targetHash)
	want := parts - 1
	if len(indexes) != want {
		t.Fatalf("expected %d matching parts for duplicate map hash, got %d", want, len(indexes))
	}

	for i, idx := range indexes {
		if idx != i {
			t.Fatalf("expected sequential index %d at position %d, got %d", i, i, idx)
		}
	}
}

func TestHashmapSegment_ReturnsExpectedSlices(t *testing.T) {
	const sdu = 384
	payload := bytes.Repeat([]byte{0x31}, 40000)

	res, err := New(payload, false)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		return append([]byte(nil), plain...), nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	entries := HashmapEntriesPerSegment(sdu)
	if entries <= 0 {
		t.Fatalf("expected positive entries per segment, got %d", entries)
	}
	totalParts := int(res.GetSegments())
	if totalParts <= entries {
		t.Fatalf("expected multi-segment hashmap, parts=%d entries=%d", totalParts, entries)
	}

	seg0 := res.HashmapSegment(sdu, 0)
	seg1 := res.HashmapSegment(sdu, 1)
	if len(seg0) == 0 || len(seg1) == 0 {
		t.Fatalf("expected non-empty segment slices, got seg0=%d seg1=%d", len(seg0), len(seg1))
	}
	if len(seg0)%MapHashLen != 0 || len(seg1)%MapHashLen != 0 {
		t.Fatalf("invalid segment lengths seg0=%d seg1=%d", len(seg0), len(seg1))
	}
	if bytes.Equal(seg0, seg1) {
		t.Fatal("expected different hashmap bytes for adjacent segments")
	}
}

func TestOutboundCiphertextViewReturnsCopy(t *testing.T) {
	const sdu = 32
	payload := bytes.Repeat([]byte{0x33}, 128)

	res, err := New(payload, false)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}
	identityEncrypt := func(plain []byte) ([]byte, error) {
		out := make([]byte, len(plain))
		copy(out, plain)
		return out, nil
	}
	if err := res.PrepareOutboundForLink(identityEncrypt, sdu); err != nil {
		t.Fatalf("PrepareOutboundForLink: %v", err)
	}

	a := res.OutboundCiphertextView(0, sdu)
	if len(a) == 0 {
		t.Fatal("empty ciphertext view")
	}
	b := res.OutboundCiphertextView(0, sdu)
	if len(b) == 0 {
		t.Fatal("empty second ciphertext view")
	}
	a[0] ^= 0xFF
	if a[0] == b[0] {
		t.Fatal("ciphertext view should not alias internal storage")
	}
}
