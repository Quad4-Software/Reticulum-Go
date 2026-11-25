// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/internal/sqlitestore"
)

func TestKnownDestinationsSQLiteCleanMarksDelete(t *testing.T) {
	if !sqlitestore.Available {
		t.Skip("sqlite driver not compiled in")
	}
	resetKnownDestinations(t)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	InitKnownDestinationsPersistence(cfgPath, false)

	id, err := New()
	if err != nil {
		t.Fatal(err)
	}
	dest := id.Hash()
	if !Remember([]byte("pkt"), dest, id.GetPublicKey(), nil) {
		t.Fatal("Remember failed")
	}
	SaveKnownDestinationsSync()
	key := knownDestKey(dest)

	knownDestinationsLock.Lock()
	meta := knownDestMetaByKey[key]
	meta.rememberedAt = time.Now().Add(-UnusedDestinationLinger - time.Second).Unix()
	meta.lastUsed = 0
	knownDestMetaByKey[key] = meta
	knownDestinationsLock.Unlock()
	result := CleanKnownDestinations(func([]byte) bool { return false })
	if result.Removed != 1 {
		t.Fatalf("clean removed=%d want 1", result.Removed)
	}
	SaveKnownDestinationsSync()

	s := knownStore.Load()
	if s == nil {
		t.Fatal("sqlite store not active")
	}
	rows, err := s.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if string(row.DestinationHash) == string(dest) {
			t.Fatal("cleaned destination remained in sqlite")
		}
	}
}
