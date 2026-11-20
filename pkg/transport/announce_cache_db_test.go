// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/internal/sqlitestore"
)

func TestOpenAnnounceCacheDBCreatesCacheDir(t *testing.T) {
	if !sqlitestore.Available {
		t.Skip("sqlite driver not compiled in")
	}
	dir := t.TempDir()
	if _, err := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(err) {
		t.Fatalf("unexpected cache dir state: %v", err)
	}
	db := openAnnounceCacheDB(dir)
	if db == nil {
		t.Fatal("expected announce cache DB on fresh storage dir")
	}
	if _, err := os.Stat(filepath.Join(dir, "cache", "announces.db")); err != nil {
		t.Fatalf("announces.db not created: %v", err)
	}
}
