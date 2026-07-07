// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestAnnounceEmitted(t *testing.T) {
	blob := []byte{0, 1, 2, 3, 4, 0, 0, 0, 0x2A, 0x10}
	if got := announceEmitted(blob); got != 0x2A10 {
		t.Fatalf("announceEmitted = %#x, want %#x", got, 0x2A10)
	}
}

func TestShouldUpdateAnnouncePathNewDestination(t *testing.T) {
	if !shouldUpdateAnnouncePath(nil, announcePathInput{destinationKnown: false}, false) {
		t.Fatal("expected new destination to update path")
	}
}

func TestShouldUpdateAnnouncePathBetterHop(t *testing.T) {
	existing := &common.Path{
		HopCount:    4,
		RandomBlobs: [][]byte{{0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
		Expires:     time.Now().Add(time.Hour),
	}
	blob := []byte{9, 9, 9, 9, 9, 0, 0, 0, 0, 50}
	if !shouldUpdateAnnouncePath(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     3,
		randomBlob:       blob,
		now:              time.Now(),
	}, false) {
		t.Fatal("expected better hop count with fresh random blob to update")
	}
}

func TestShouldUpdateAnnouncePathRejectsReplay(t *testing.T) {
	blob := []byte{1, 2, 3, 4, 5, 0, 0, 0, 0, 9}
	existing := &common.Path{
		HopCount:    3,
		RandomBlobs: [][]byte{append([]byte(nil), blob...)},
		Expires:     time.Now().Add(time.Hour),
	}
	if shouldUpdateAnnouncePath(existing, announcePathInput{
		destinationKnown: true,
		announceHops:     3,
		randomBlob:       blob,
		now:              time.Now(),
	}, false) {
		t.Fatal("expected known random blob replay to be rejected")
	}
}

func TestAppendRandomBlobCaps(t *testing.T) {
	var blobs [][]byte
	for i := range maxRandomBlobs + 5 {
		blob := bytes.Repeat([]byte{byte(i)}, 10)
		blobs = appendRandomBlob(blobs, blob)
	}
	if len(blobs) != maxRandomBlobs {
		t.Fatalf("len(blobs)=%d want %d", len(blobs), maxRandomBlobs)
	}
}
