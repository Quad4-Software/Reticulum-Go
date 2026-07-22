// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// retainSurviveOracle checks that a retained destination survives concurrent cleans.
type retainSurviveOracle struct {
	Survived bool
}

func TestOracleTOCTOURetainSurvivesConcurrentClean(t *testing.T) {
	resetKnownForTest(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(append([]byte{0x70}, id.GetPublicKey()...))
	Remember([]byte("retain"), dest, id.GetPublicKey(), nil)
	RetainKnownDestination(dest)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[knownDestKey(dest)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Hour).Unix()
	knownDestMetaByKey[knownDestKey(dest)] = meta
	knownDestinationsLock.Unlock()

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 30 {
				RetainKnownDestination(dest)
				_ = CleanKnownDestinations(func([]byte) bool { return false })
			}
		})
	}
	wg.Wait()

	_, err = Recall(dest)
	oracle := retainSurviveOracle{Survived: err == nil}
	if !oracle.Survived {
		t.Fatal("retained destination deleted under concurrent clean (TOCTOU)")
	}
}

func TestAdversarialTOCTOUTouchBeatsStaleCleanVerdict(t *testing.T) {
	resetKnownForTest(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(append([]byte{0x71}, id.GetPublicKey()...))
	Remember([]byte("touch"), dest, id.GetPublicKey(), nil)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[knownDestKey(dest)]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Hour).Unix()
	meta.lastUsed = 0
	knownDestMetaByKey[knownDestKey(dest)] = meta
	knownDestinationsLock.Unlock()

	// Establish a successful Touch before cleans race. A clean that wins
	// entirely before any Touch is a legitimate delete, not a TOCTOU bug.
	TouchKnownDestination(dest)

	var wg sync.WaitGroup
	barrier := make(chan struct{})
	for range 12 {
		wg.Go(func() {
			<-barrier
			for range 40 {
				TouchKnownDestination(dest)
			}
		})
		wg.Go(func() {
			<-barrier
			for range 40 {
				_ = CleanKnownDestinations(func([]byte) bool { return false })
			}
		})
	}
	close(barrier)
	wg.Wait()

	if _, err := Recall(dest); err != nil {
		t.Fatal("touched destination must survive concurrent clean")
	}
}

func TestOracleTOCTOUTouchInvalidatesStaleCleanVerdict(t *testing.T) {
	resetKnownForTest(t)
	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(append([]byte{0x72}, id.GetPublicKey()...))
	Remember([]byte("touch2"), dest, id.GetPublicKey(), nil)
	key := knownDestKey(dest)
	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[key]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Hour).Unix()
	meta.lastUsed = 0
	knownDestMetaByKey[key] = meta
	knownDestinationsLock.Unlock()

	now := time.Now()
	nowUnix := now.Unix()
	noPath := func([]byte) bool { return false }

	knownDestinationsLock.Lock()
	if !stillStaleForCleanLocked(key, noPath, now, nowUnix) {
		knownDestinationsLock.Unlock()
		t.Fatal("fixture must be stale before Touch")
	}
	knownDestinationsLock.Unlock()

	TouchKnownDestination(dest)

	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()
	if stillStaleForCleanLocked(key, noPath, now, nowUnix) {
		t.Fatal("Touch must invalidate stale clean verdict under write lock")
	}
}

func TestOracleTOCTOUKnownPersistDirtyGen(t *testing.T) {
	prevMem := knownPersistMemory.Load()
	prevDis := knownPersistDisabled.Load()
	prevDirty := knownPersistDirty.Load()
	prevGen := knownPersistGen.Load()
	t.Cleanup(func() {
		knownPersistMemory.Store(prevMem)
		knownPersistDisabled.Store(prevDis)
		knownPersistDirty.Store(prevDirty)
		knownPersistGen.Store(prevGen)
	})

	knownPersistMemory.Store(false)
	knownPersistDisabled.Store(false)
	knownPersistDirty.Store(false)
	knownPersistGen.Store(0)

	markKnownDestinationsDirty()
	genAtSnapshot := knownPersistGen.Load()
	markKnownDestinationsDirty()
	// Stale clear must not drop dirty when gen advanced after snapshot.
	if knownPersistGen.Load() == genAtSnapshot {
		knownPersistDirty.Store(false)
	}
	if !knownPersistDirty.Load() {
		t.Fatal("dirty cleared with stale snapshot gen (TOCTOU)")
	}
	if knownPersistGen.Load() == genAtSnapshot {
		t.Fatal("expected gen to advance on concurrent mark")
	}
}

func TestAdversarialTOCTOUKnownPersistMarkDuringSave(t *testing.T) {
	tmp := t.TempDir()
	InitKnownDestinationsPersistence(tmp, false)
	t.Cleanup(func() {
		InitKnownDestinationsPersistence("", true)
	})

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := TruncatedHash(id.GetPublicKey())
	Remember([]byte("persist"), dest, id.GetPublicKey(), nil)

	var wg sync.WaitGroup
	var marks atomic.Int32
	for range 20 {
		wg.Go(func() {
			for j := range 25 {
				Remember([]byte{byte(j)}, TruncatedHash(append([]byte{byte(j)}, id.GetPublicKey()...)), id.GetPublicKey(), nil)
				marks.Add(1)
				PersistKnownDestinationsIfDirty()
			}
		})
	}
	wg.Wait()
	SaveKnownDestinationsSync()
	if marks.Load() == 0 {
		t.Fatal("expected Remember traffic")
	}
}
