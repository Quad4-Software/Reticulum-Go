// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/internal/storage"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

// Guarantee: persistPathTableIfDirty honors PathPersistMinInterval, while
// savePathTableSync / Close force a write even inside the window.
func TestOraclePathPersistThrottleAndForce(t *testing.T) {
	if PathPersistMinInterval != 12*time.Hour {
		t.Fatalf("PathPersistMinInterval=%v want 12h (RNS PERSIST_INTERVAL)", PathPersistMinInterval)
	}
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	cfg := &common.ReticulumConfig{ConfigPath: cfgPath}

	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	iface := newPersistMockInterface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0xA1}, identity.TruncatedHashLength/8)
	next := bytes.Repeat([]byte{0xB2}, identity.TruncatedHashLength/8)
	tr.UpdatePath(dest, next, "wan", 2)
	tr.savePathTableSync()

	path, err := storage.DestinationTablePath(cfgPath)
	if err != nil {
		t.Fatalf("DestinationTablePath: %v", err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	tr.UpdatePath(dest, next, "wan", 3)
	if !tr.pathPersistDirty.Load() {
		t.Fatal("expected dirty after UpdatePath")
	}
	tr.pathPersistLast.Store(time.Now().UnixNano())
	tr.persistPathTableIfDirty()
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat2: %v", err)
	}
	if !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatal("throttled dirty path flush wrote destination_table")
	}
	if !tr.pathPersistDirty.Load() {
		t.Fatal("dirty must remain when throttle skips")
	}

	tr.savePathTableSync()
	if tr.pathPersistDirty.Load() {
		t.Fatal("force save must clear dirty")
	}

	tr2 := NewTransport(cfg)
	t.Cleanup(func() { _ = tr2.Close() })
	if err := tr2.RegisterInterface("wan", newPersistMockInterface("wan")); err != nil {
		t.Fatal(err)
	}
	tr2.mutex.RLock()
	p := tr2.paths[pathMapKey(dest)]
	tr2.mutex.RUnlock()
	if p == nil || p.HopCount != 3 {
		t.Fatalf("forced path persist lost hop count, path=%v", p)
	}
}

func TestOracleClientDetachPersistBypassesIntervalOnlyWhenDirty(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config")
	tr := NewTransport(&common.ReticulumConfig{ConfigPath: cfgPath})
	t.Cleanup(func() { _ = tr.Close() })
	if err := tr.RegisterInterface("wan", newPersistMockInterface("wan")); err != nil {
		t.Fatal(err)
	}
	dest := bytes.Repeat([]byte{0xC3}, identity.TruncatedHashLength/8)
	next := bytes.Repeat([]byte{0xD4}, identity.TruncatedHashLength/8)
	tr.UpdatePath(dest, next, "wan", 1)
	tr.pathPersistLast.Store(time.Now().UnixNano())
	tr.PersistStateForClientDetach()
	if tr.pathPersistDirty.Load() {
		t.Fatal("client-detach persist did not clear dirty path table")
	}
	path, err := storage.DestinationTablePath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("client-detach persist did not write destination table: %v", err)
	}
}
