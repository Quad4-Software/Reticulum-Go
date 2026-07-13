// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"strings"
	"testing"

	"quad4/reticulum-go/pkg/resource"
)

func makeFakeHashmap(parts int) []byte {
	out := make([]byte, parts*resource.MapHashLen)
	for i := range out {
		out[i] = byte(i)
	}
	return out
}

func TestBeginIncomingResource_RejectsPartsBomb(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        int(resource.MaxSegments) + 1,
		TransferSize: 1024,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x01}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x02}, 32),
		Hashmap:      makeFakeHashmap(1),
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "MaxSegments") {
		t.Fatalf("expected MaxSegments rejection, got: %v", err)
	}
}

func TestBeginIncomingResource_RejectsHugeIntPartsBomb(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        1_000_000_000,
		TransferSize: 1024,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x03}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x04}, 32),
		Hashmap:      makeFakeHashmap(1),
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "MaxSegments") {
		t.Fatalf("expected MaxSegments rejection for 1B parts, got: %v", err)
	}
}

func TestBeginIncomingResource_RejectsNegativeTransferSize(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: -1,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x05}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x06}, 32),
		Hashmap:      makeFakeHashmap(4),
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "negative transfer_size") {
		t.Fatalf("expected negative transfer_size rejection, got: %v", err)
	}
}

func TestBeginIncomingResource_RejectsTransferSizeBeyondPartsCapacity(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 1024 * 1024 * 1024,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x07}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x08}, 32),
		Hashmap:      makeFakeHashmap(4),
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "transfer_size exceeds parts*sdu") {
		t.Fatalf("expected oversized-transfer rejection, got: %v", err)
	}
}

func TestBeginIncomingResource_RejectsZeroParts(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        0,
		TransferSize: 1024,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x09}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x0A}, 32),
		Hashmap:      makeFakeHashmap(1),
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "invalid parts") {
		t.Fatalf("expected invalid-parts rejection, got: %v", err)
	}
}

func TestBeginIncomingResource_RejectsBadHashmap(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 100,
		DataSize:     400,
		RandomHash:   bytes.Repeat([]byte{0x0B}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x0C}, 32),
		Hashmap:      []byte{0x01, 0x02, 0x03},
	}

	if err := l.beginIncomingResource(adv); err == nil ||
		!strings.Contains(err.Error(), "hashmap") {
		t.Fatalf("expected hashmap rejection, got: %v", err)
	}
}

func TestBeginIncomingResource_AcceptsHonestAdvertisement(t *testing.T) {
	l := &Link{}
	l.mdu = 384

	adv := &resource.ResourceAdvertisement{
		Parts:        4,
		TransferSize: 4 * 384,
		DataSize:     1024,
		RandomHash:   bytes.Repeat([]byte{0x0D}, resource.RandomHashSize),
		Hash:         bytes.Repeat([]byte{0x0E}, 32),
		Hashmap:      makeFakeHashmap(1),
	}

	defer l.resetIncomingResource()

	err := l.beginIncomingResource(adv)
	if err != nil && !strings.Contains(err.Error(), "link not active") {
		t.Fatalf("honest advertisement rejected: %v", err)
	}
}
