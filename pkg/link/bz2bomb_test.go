// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"quad4/bzip2/pkg/bzip2"
	"quad4/reticulum-go/pkg/resource"
)

func bz2Stream(t *testing.T, plaintext []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		t.Fatalf("bzip2.NewWriter: %v", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		t.Fatalf("bzip2 write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 close: %v", err)
	}
	return buf.Bytes()
}

func bz2Bomb(t *testing.T, decompressedLen int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, 9)
	if err != nil {
		t.Fatalf("bzip2.NewWriter: %v", err)
	}
	zeros := make([]byte, 64*1024)
	remaining := decompressedLen
	for remaining > 0 {
		n := min(len(zeros), remaining)
		if _, err := w.Write(zeros[:n]); err != nil {
			t.Fatalf("bzip2 write: %v", err)
		}
		remaining -= n
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 close: %v", err)
	}
	return buf.Bytes()
}

func compressedAdv(t *testing.T, plaintext []byte) ([]byte, *resource.ResourceAdvertisement) {
	t.Helper()
	randomHash := bytes.Repeat([]byte{0x12}, resource.RandomHashSize)
	compressed := bz2Stream(t, plaintext)
	inner := append(append([]byte(nil), randomHash...), compressed...)
	sum := sha256.Sum256(append(append([]byte(nil), plaintext...), randomHash...))
	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   int64(len(plaintext)),
		RandomHash: randomHash,
		Hash:       sum[:],
	}
	return inner, adv
}

func plaintextAdv(plaintext []byte) ([]byte, *resource.ResourceAdvertisement) {
	randomHash := bytes.Repeat([]byte{0x34}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), plaintext...)
	sum := sha256.Sum256(append(append([]byte(nil), plaintext...), randomHash...))
	adv := &resource.ResourceAdvertisement{
		Compressed: false,
		Encrypted:  false,
		DataSize:   int64(len(plaintext)),
		RandomHash: randomHash,
		Hash:       sum[:],
	}
	return inner, adv
}

func TestAssembleIncomingPayload_RejectsBz2Bomb(t *testing.T) {
	const claimedSmallSize = 1024
	const actualHugeSize = 256 * 1024 * 1024

	bomb := bz2Bomb(t, actualHugeSize)
	if len(bomb) > 4096 {
		t.Fatalf("bomb stream unexpectedly large: %d bytes", len(bomb))
	}
	t.Logf("bomb stream: %d bytes compressed -> %d bytes decompressed (ratio %dx)",
		len(bomb), actualHugeSize, actualHugeSize/len(bomb))

	randomHash := bytes.Repeat([]byte{0xAB}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), bomb...)

	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   claimedSmallSize,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	out, err := l.assembleIncomingPayload(inner, adv)
	if err == nil {
		t.Fatalf("expected bomb to be rejected, got %d bytes back", len(out))
	}
	if !strings.Contains(err.Error(), "exceeds advertised data_size") {
		t.Fatalf("expected size-cap error, got: %v", err)
	}
}

func TestAssembleIncomingPayload_RejectsAutoCompressMaxSize(t *testing.T) {
	tinyBomb := bz2Bomb(t, 1024)
	randomHash := bytes.Repeat([]byte{0xCD}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), tinyBomb...)

	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   int64(resource.AutoCompressMaxSize) + 1,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	out, err := l.assembleIncomingPayload(inner, adv)
	if err == nil {
		t.Fatalf("expected oversized data_size to be rejected, got %d bytes", len(out))
	}
	if !strings.Contains(err.Error(), "AutoCompressMaxSize") {
		t.Fatalf("expected AutoCompressMaxSize error, got: %v", err)
	}
}

func TestAssembleIncomingPayload_RejectsZeroDataSize(t *testing.T) {
	tinyBomb := bz2Bomb(t, 8)
	randomHash := bytes.Repeat([]byte{0xEF}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), tinyBomb...)

	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   0,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	if _, err := l.assembleIncomingPayload(inner, adv); err == nil ||
		!strings.Contains(err.Error(), "data_size") {
		t.Fatalf("expected data_size rejection, got: %v", err)
	}
}

func TestAssembleIncomingPayload_RejectsNegativeDataSize(t *testing.T) {
	tinyBomb := bz2Bomb(t, 8)
	randomHash := bytes.Repeat([]byte{0xFE}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), tinyBomb...)

	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   -1,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	if _, err := l.assembleIncomingPayload(inner, adv); err == nil ||
		!strings.Contains(err.Error(), "data_size") {
		t.Fatalf("expected negative data_size rejection, got: %v", err)
	}
}

func TestAssembleIncomingPayload_RejectsCorruptBz2Stream(t *testing.T) {
	corrupt := []byte("not actually a bzip2 stream, just garbage")
	randomHash := bytes.Repeat([]byte{0x77}, resource.RandomHashSize)
	inner := append(append([]byte(nil), randomHash...), corrupt...)

	adv := &resource.ResourceAdvertisement{
		Compressed: true,
		Encrypted:  false,
		DataSize:   1024,
		RandomHash: randomHash,
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	if _, err := l.assembleIncomingPayload(inner, adv); err == nil {
		t.Fatalf("expected corrupt bz2 stream to be rejected")
	}
}

func TestAssembleIncomingPayload_AcceptsHonestCompressedAtCap(t *testing.T) {
	plaintext := bytes.Repeat([]byte("reticulum-bz2-acceptance-payload"), 8192)
	if len(plaintext) > resource.AutoCompressMaxSize {
		t.Fatalf("test fixture exceeds AutoCompressMaxSize")
	}

	inner, adv := compressedAdv(t, plaintext)
	l := &Link{}
	out, err := l.assembleIncomingPayload(inner, adv)
	if err != nil {
		t.Fatalf("honest compressed payload rejected: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(out), len(plaintext))
	}
}

func TestAssembleIncomingPayload_AcceptsHonestPlaintext(t *testing.T) {
	plaintext := []byte("plain bytes, no compression flag")
	inner, adv := plaintextAdv(plaintext)

	l := &Link{}
	out, err := l.assembleIncomingPayload(inner, adv)
	if err != nil {
		t.Fatalf("honest plaintext payload rejected: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("plaintext mismatch")
	}
}

func TestAssembleIncomingPayload_RejectsHashMismatchOnHonestCompressed(t *testing.T) {
	plaintext := []byte("genuine but advertisement-tampered")
	inner, adv := compressedAdv(t, plaintext)
	adv.Hash = bytes.Repeat([]byte{0xAA}, sha256.Size)

	l := &Link{}
	if _, err := l.assembleIncomingPayload(inner, adv); err == nil ||
		!strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch error, got: %v", err)
	}
}

func TestAssembleIncomingPayload_RejectsUndersizedInner(t *testing.T) {
	short := []byte{0x01, 0x02}
	adv := &resource.ResourceAdvertisement{
		Compressed: false,
		Encrypted:  false,
		DataSize:   2,
		RandomHash: bytes.Repeat([]byte{0x00}, resource.RandomHashSize),
		Hash:       bytes.Repeat([]byte{0x00}, sha256.Size),
	}

	l := &Link{}
	if _, err := l.assembleIncomingPayload(short, adv); err == nil ||
		!strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected too-short error, got: %v", err)
	}
}
