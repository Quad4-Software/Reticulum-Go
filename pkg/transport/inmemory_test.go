// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestInMemoryStorage_NoDiskWrites(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &common.ReticulumConfig{
		ConfigPath:       filepath.Join(home, ".reticulum-go", "config"),
		InMemoryStorage:  true,
		EnableTransport:  true,
		ShareInstance:    false,
		MaxInMemoryPaths: 100,
	}
	tr := NewTransport(cfg)
	t.Cleanup(func() { identity.SetKnownDestinationsMaxEntries(0) })

	if !tr.pathPersistMemory.Load() {
		t.Fatal("path table should be in-memory")
	}
	if tr.BlackholeTable() == nil {
		t.Fatal("expected RAM blackhole table")
	}
	if tr.BlackholeTable().Dir() != "" {
		t.Fatal("blackhole dir should be empty in memory mode")
	}
	if h := tr.TransportIdentityHash(); len(h) == 0 {
		t.Fatal("expected ephemeral transport identity")
	}

	storageDir := filepath.Join(home, ".reticulum-go", "storage")
	if _, err := os.Stat(storageDir); !os.IsNotExist(err) {
		t.Fatalf("in-memory mode must not create storage dir, err=%v", err)
	}
}

func TestInMemoryStorage_PathEviction(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	cfg := &common.ReticulumConfig{
		InMemoryStorage:  true,
		EnableTransport:  true,
		ShareInstance:    false,
		MaxInMemoryPaths: 3,
	}
	tr := NewTransport(cfg)
	t.Cleanup(func() { identity.SetKnownDestinationsMaxEntries(0) })

	iface := newPersistMockInterface("udp0")
	if err := tr.RegisterInterface("udp0", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	now := time.Now()
	tr.mutex.Lock()
	for i := range 5 {
		h := make([]byte, PathMapKeySize)
		h[0] = byte(i + 1)
		nh := make([]byte, PathMapKeySize)
		nh[0] = byte(i + 10)
		tr.updatePathUnlocked(h, nh, "udp0", 1, nil, nil, now.Add(time.Duration(i)*time.Second))
	}
	n := len(tr.paths)
	for i := range 2 {
		h := make([]byte, PathMapKeySize)
		h[0] = byte(i + 1)
		if _, ok := tr.paths[pathMapKey(h)]; ok {
			tr.mutex.Unlock()
			t.Fatalf("oldest path %d should have been evicted", i+1)
		}
	}
	tr.mutex.Unlock()
	if n != 3 {
		t.Fatalf("paths = %d, want 3 after eviction", n)
	}
}

func TestEmptyConfigPath_EphemeralIdentity(t *testing.T) {
	t.Setenv("RETICULUM_STORAGE_PATH", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	_ = NewTransport(&common.ReticulumConfig{ShareInstance: false})
	t.Cleanup(func() { identity.SetKnownDestinationsMaxEntries(0) })

	if _, err := os.Stat(filepath.Join(home, ".reticulum")); !os.IsNotExist(err) {
		t.Fatal("must not create ~/.reticulum for empty config path")
	}
	if _, err := os.Stat(filepath.Join(home, ".reticulum-go")); !os.IsNotExist(err) {
		t.Fatal("must not create ~/.reticulum-go for empty config path")
	}
}
