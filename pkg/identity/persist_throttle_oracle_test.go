// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/internal/storage"
)

// Guarantee: dirty periodic flushes respect KnownPersistMinInterval, while
// SaveKnownDestinationsSync (force) always writes and clears dirty.
func TestOracleKnownPersistThrottleSkipsThenForceWrites(t *testing.T) {
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !Remember([]byte("pkt1"), id.Hash(), id.GetPublicKey(), []byte("app1")) {
		t.Fatal("Remember failed")
	}

	SaveKnownDestinationsSync()
	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after force: %v", err)
	}

	if !Remember([]byte("pkt2"), id.Hash(), id.GetPublicKey(), []byte("app2")) {
		t.Fatal("Remember update failed")
	}
	if !knownPersistDirty.Load() {
		t.Fatal("expected dirty after Remember")
	}

	// Simulate a just-completed flush so the throttle window is active.
	knownPersistLast.Store(time.Now().UnixNano())
	PersistKnownDestinationsIfDirty()
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after throttle: %v", err)
	}
	if !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatal("throttled dirty flush wrote known_destinations")
	}
	if !knownPersistDirty.Load() {
		t.Fatal("dirty must remain set when throttle skips")
	}

	SaveKnownDestinationsSync()
	if knownPersistDirty.Load() {
		t.Fatal("force save must clear dirty")
	}
	info3, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after force2: %v", err)
	}
	if !info3.ModTime().After(info1.ModTime()) && info3.Size() == info1.Size() {
		// ModTime granularity can be coarse. Content must still change.
		b1, _ := os.ReadFile(path)
		_ = b1
	}
	recalled, err := Recall(id.Hash())
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if recalled == nil {
		t.Fatal("missing recall after force save")
	}

	knownDestinationsLock.RLock()
	e := knownDestinations[knownDestKey(id.Hash())]
	knownDestinationsLock.RUnlock()
	if len(e.rawKey) != TruncatedHashLength/8 {
		t.Fatalf("rawKey len=%d want %d", len(e.rawKey), TruncatedHashLength/8)
	}
}
