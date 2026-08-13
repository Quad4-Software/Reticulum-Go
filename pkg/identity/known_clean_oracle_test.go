// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"sync"
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
)

func resetKnownForTest(t *testing.T) {
	t.Helper()
	knownDestinationsLock.Lock()
	prevDest := knownDestinations
	prevMeta := knownDestMetaByKey
	knownDestinations = make(map[destMapKey][]any)
	knownDestMetaByKey = make(map[destMapKey]knownDestMeta)
	knownDestinationsLock.Unlock()
	t.Cleanup(func() {
		knownDestinationsLock.Lock()
		knownDestinations = prevDest
		knownDestMetaByKey = prevMeta
		knownDestinationsLock.Unlock()
	})
}

func TestOracleCleanKnownDestinationsMatrix(t *testing.T) {
	resetKnownForTest(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	fresh := TruncatedHash(append([]byte{0x01}, id.GetPublicKey()...))
	stale := TruncatedHash(append([]byte{0x02}, id.GetPublicKey()...))
	pathful := TruncatedHash(append([]byte{0x03}, id.GetPublicKey()...))
	retained := TruncatedHash(append([]byte{0x04}, id.GetPublicKey()...))

	Remember([]byte("a"), fresh, id.GetPublicKey(), nil)
	Remember([]byte("b"), stale, id.GetPublicKey(), nil)
	Remember([]byte("c"), pathful, id.GetPublicKey(), nil)
	Remember([]byte("d"), retained, id.GetPublicKey(), nil)
	RetainKnownDestination(retained)

	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[knownDestKey(stale)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	meta.lastUsed = 0
	knownDestMetaByKey[knownDestKey(stale)] = meta
	meta = knownDestMetaByKey[knownDestKey(pathful)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	knownDestMetaByKey[knownDestKey(pathful)] = meta
	meta = knownDestMetaByKey[knownDestKey(retained)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	knownDestMetaByKey[knownDestKey(retained)] = meta
	knownDestinationsLock.Unlock()

	pathKey := knownDestKey(pathful)
	res := CleanKnownDestinations(func(h []byte) bool {
		return knownDestKey(h) == pathKey
	})
	if res.Removed != 1 {
		t.Fatalf("removed=%d want 1 (only stale never-used)", res.Removed)
	}
	if _, err := Recall(fresh); err != nil {
		t.Fatal("fresh never-used must survive")
	}
	if _, err := Recall(pathful); err != nil {
		t.Fatal("pathful must survive")
	}
	if _, err := Recall(retained); err != nil {
		t.Fatal("retained must survive")
	}
	if _, err := Recall(stale); err == nil {
		t.Fatal("stale never-used must be removed")
	}
}

func TestAdversarialCleanKnownDestinationsNilHasPath(t *testing.T) {
	resetKnownForTest(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("x"), dest, id.GetPublicKey(), nil)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[knownDestKey(dest)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Minute).Unix()
	knownDestMetaByKey[knownDestKey(dest)] = meta
	knownDestinationsLock.Unlock()

	res := CleanKnownDestinations(nil)
	if res.Removed != 1 {
		t.Fatalf("nil hasPath should treat as pathless, removed=%d", res.Removed)
	}
}

func TestPBTCleanKnownNeverRemovesRetained(t *testing.T) {
	ageMin := pbt.IntRange(1, 20)
	prop := pbt.ForAll(
		"retained destinations survive cleaning regardless of age",
		ageMin,
		func(age int) bool {
			reset := func() (cleanup func()) {
				knownDestinationsLock.Lock()
				prevDest := knownDestinations
				prevMeta := knownDestMetaByKey
				knownDestinations = make(map[destMapKey][]any)
				knownDestMetaByKey = make(map[destMapKey]knownDestMeta)
				knownDestinationsLock.Unlock()
				return func() {
					knownDestinationsLock.Lock()
					knownDestinations = prevDest
					knownDestMetaByKey = prevMeta
					knownDestinationsLock.Unlock()
				}
			}
			cleanup := reset()
			defer cleanup()
			id, err := New()
			if err != nil {
				return false
			}
			dest := TruncatedHash(id.GetPublicKey())
			Remember([]byte("r"), dest, id.GetPublicKey(), nil)
			RetainKnownDestination(dest)
			knownDestinationsLock.Lock()
			meta := knownDestMetaByKey[knownDestKey(dest)]
			meta.rememberedAt = time.Now().Add(-time.Duration(age) * time.Hour).Unix()
			knownDestMetaByKey[knownDestKey(dest)] = meta
			knownDestinationsLock.Unlock()
			res := CleanKnownDestinations(func([]byte) bool { return false })
			return res.Removed == 0
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(12), pbt.WithSeed(145))
}

func TestRaceKnownDestCleanAndTouch(t *testing.T) {
	resetKnownForTest(t)
	ids := make([]*Identity, 8)
	dests := make([][]byte, 8)
	for i := range ids {
		id, err := New()
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		dests[i] = TruncatedHash(append([]byte{byte(i)}, id.GetPublicKey()...))
		Remember([]byte{byte(i)}, dests[i], id.GetPublicKey(), nil)
	}

	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			for j := range 50 {
				TouchKnownDestination(dests[j%len(dests)])
				RetainKnownDestination(dests[(j+1)%len(dests)])
				_ = CleanKnownDestinations(func([]byte) bool { return j%3 == 0 })
			}
		})
	}
	wg.Wait()
}
