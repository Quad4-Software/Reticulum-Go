// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

// Guarantee: persistPathTableIfDirty honors PathPersistMinInterval, while
// savePathTableSync / Close force a write even inside the window.
func TestOraclePathPersistThrottleAndForce(t *testing.T) {
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
