// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/internal/sqlitestore"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
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
	t.Cleanup(func() { _ = db.Close() })
	if _, err := os.Stat(filepath.Join(dir, "cache", "announces.db")); err != nil {
		t.Fatalf("announces.db not created: %v", err)
	}
}

func TestMaintainAnnounceCacheSeparatesFlushAndClean(t *testing.T) {
	if !sqlitestore.Available {
		t.Skip("sqlite driver not compiled in")
	}
	dir := t.TempDir()
	db := openAnnounceCacheDB(dir)
	if db == nil {
		t.Fatal("expected announce cache DB")
	}
	t.Cleanup(func() { _ = db.Close() })
	tr := &Transport{announceCacheDB: db, paths: make(map[[PathMapKeySize]byte]*common.Path)}
	now := time.Now()
	tr.announceCacheLastClean = now
	tr.announceCacheLastFlush = now.Add(-announceCacheFlushInterval)

	hash := bytes.Repeat([]byte{0x42}, 32)
	db.Store(hash, []byte("announce"), "wan")
	tr.maintainAnnounceCacheDB()
	if got := db.PendingCount(); got != 0 {
		t.Fatalf("flush interval did not write pending rows, pending=%d", got)
	}
	if _, _, ok := db.Fetch(hash); !ok {
		t.Fatal("flushed announce missing")
	}

	// The inactive row must survive the 15 second flush pass. Cleaning is a
	// separate five minute operation in Python.
	tr.announceCacheLastFlush = time.Now().Add(-announceCacheFlushInterval)
	tr.maintainAnnounceCacheDB()
	if _, _, ok := db.Fetch(hash); !ok {
		t.Fatal("flush pass incorrectly cleaned inactive announce")
	}
	tr.announceCacheLastClean = time.Now().Add(-announceCacheCleanInterval)
	tr.maintainAnnounceCacheDB()
	if _, _, ok := db.Fetch(hash); ok {
		t.Fatal("clean pass retained announce with no active path")
	}
}
