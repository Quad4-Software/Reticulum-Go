// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/internal/sqlitestore"
)

// Standalone GetRatchet (no transport init in this "process") must still
// recover a ratchet persisted in known_destinations.db, matching the legacy
// file backend which resolved the storage path lazily on every read.
func TestGetRatchetLazyOpensKnownStore(t *testing.T) {
	if !sqlitestore.Available {
		t.Skip("sqlite store unavailable on this platform")
	}
	dir := t.TempDir()
	t.Setenv("RETICULUM_STORAGE_PATH", dir)
	t.Setenv("RETICULUM_DISABLE_SQLITE_KNOWN", "")

	InitKnownDestinationsPersistence("", false)
	destHash := bytes.Repeat([]byte{0xAB}, TruncatedHashLength/8)
	ratchet := bytes.Repeat([]byte{0xCD}, RatchetSize/8)
	RememberRatchet(destHash, ratchet)
	CloseKnownDestinationsStore()

	// Simulate a fresh process: empty in-memory tables, store not yet open.
	ratchetPersistLock.Lock()
	knownRatchets = make(map[destMapKey]knownRatchetEntry)
	ratchetPersistLock.Unlock()
	knownStore.Store(nil)
	knownStoreReadInit.Store(false)
	t.Cleanup(func() {
		CloseKnownDestinationsStore()
		InitKnownDestinationsPersistence("", true)
	})

	got := GetRatchet(destHash)
	if !bytes.Equal(got, ratchet) {
		t.Fatalf("GetRatchet = %x, want %x", got, ratchet)
	}
}
