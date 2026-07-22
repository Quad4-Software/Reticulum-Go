// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"testing"
	"time"
)

func TestCleanKnownDestinationsRemovesStaleNeverUsed(t *testing.T) {
	knownDestinationsLock.Lock()
	prevDest := knownDestinations
	prevMeta := knownDestMetaByKey
	knownDestinations = make(map[string][]any)
	knownDestMetaByKey = make(map[string]knownDestMeta)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prevDest
		knownDestMetaByKey = prevMeta
		knownDestinationsLock.Unlock()
	})

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("pkt"), dest, id.GetPublicKey(), nil)

	key := knownDestKey(dest)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[key]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	meta.lastUsed = 0
	knownDestMetaByKey[key] = meta
	knownDestinationsLock.Unlock()

	res := CleanKnownDestinations(func([]byte) bool { return false })
	if res.Removed != 1 {
		t.Fatalf("removed=%d want 1 (total=%d no_path=%d)", res.Removed, res.Total, res.NoPath)
	}
}

func TestCleanKnownDestinationsKeepsPathPresent(t *testing.T) {
	knownDestinationsLock.Lock()
	prevDest := knownDestinations
	prevMeta := knownDestMetaByKey
	knownDestinations = make(map[string][]any)
	knownDestMetaByKey = make(map[string]knownDestMeta)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prevDest
		knownDestMetaByKey = prevMeta
		knownDestinationsLock.Unlock()
	})

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("pkt"), dest, id.GetPublicKey(), nil)

	key := knownDestKey(dest)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[key]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	knownDestMetaByKey[key] = meta
	knownDestinationsLock.Unlock()

	res := CleanKnownDestinations(func(h []byte) bool {
		return knownDestKey(h) == key
	})
	if res.Removed != 0 {
		t.Fatalf("removed=%d want 0 for pathful dest", res.Removed)
	}
}

func TestCleanKnownDestinationsKeepsRetained(t *testing.T) {
	knownDestinationsLock.Lock()
	prevDest := knownDestinations
	prevMeta := knownDestMetaByKey
	knownDestinations = make(map[string][]any)
	knownDestMetaByKey = make(map[string]knownDestMeta)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prevDest
		knownDestMetaByKey = prevMeta
		knownDestinationsLock.Unlock()
	})

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("pkt"), dest, id.GetPublicKey(), nil)
	RetainKnownDestination(dest)

	key := knownDestKey(dest)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[key]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	knownDestMetaByKey[key] = meta
	knownDestinationsLock.Unlock()

	res := CleanKnownDestinations(func([]byte) bool { return false })
	if res.Removed != 0 {
		t.Fatalf("removed=%d want 0 for retained dest", res.Removed)
	}
}

func TestCleanKnownDestinationsKeepsFreshNeverUsed(t *testing.T) {
	knownDestinationsLock.Lock()
	prevDest := knownDestinations
	prevMeta := knownDestMetaByKey
	knownDestinations = make(map[string][]any)
	knownDestMetaByKey = make(map[string]knownDestMeta)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prevDest
		knownDestMetaByKey = prevMeta
		knownDestinationsLock.Unlock()
	})

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("pkt"), dest, id.GetPublicKey(), nil)

	res := CleanKnownDestinations(func([]byte) bool { return false })
	if res.Removed != 0 {
		t.Fatalf("removed=%d want 0 for fresh never-used dest", res.Removed)
	}
}
