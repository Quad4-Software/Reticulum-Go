// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
)

func TestOracleTOCTOUPathPersistDirtyGen(t *testing.T) {
	tmp := t.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	iface := newPersistMockInterface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	tr.markPathTableDirty()
	genAtSnapshot := tr.pathPersistGen.Load()
	tr.markPathTableDirty()
	if tr.pathPersistGen.Load() == genAtSnapshot {
		tr.pathPersistDirty.Store(false)
	}
	if !tr.pathPersistDirty.Load() {
		t.Fatal("dirty cleared with stale snapshot gen (TOCTOU)")
	}
}

func TestAdversarialTOCTOUPathPersistMarkDuringSave(t *testing.T) {
	tmp := t.TempDir()
	cfg := &common.ReticulumConfig{ConfigPath: filepath.Join(tmp, "config")}
	tr := NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	iface := newPersistMockInterface("wan")
	if err := tr.RegisterInterface("wan", iface); err != nil {
		t.Fatal(err)
	}

	dest := bytes.Repeat([]byte{0x11}, identity.TruncatedHashLength/8)
	next := bytes.Repeat([]byte{0x22}, identity.TruncatedHashLength/8)
	tr.UpdatePath(dest, next, "wan", 1)

	var wg sync.WaitGroup
	var updates atomic.Int32
	for i := range 16 {
		hop := uint8(i + 1)
		wg.Go(func() {
			for j := range 20 {
				d := bytes.Repeat([]byte{byte(j + 1)}, identity.TruncatedHashLength/8)
				tr.UpdatePath(d, next, "wan", hop)
				updates.Add(1)
				tr.persistPathTableIfDirty()
			}
		})
	}
	wg.Wait()
	tr.savePathTableSync()
	if updates.Load() == 0 {
		t.Fatal("expected path updates")
	}
	if tr.pathPersistDirty.Load() {
		t.Fatal("dirty stuck true after quiescent sync save")
	}
}

func TestAdversarialTOCTOUKnownDestCleanSingleFlight(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{})
	t.Cleanup(func() { _ = tr.Close() })
	tr.destinationsLastCleaned.Store(0)

	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			tr.maybeCleanKnownDestinations()
		})
	}
	wg.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !tr.knownDestCleaning.Load() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("knownDestCleaning stuck true after concurrent maybeClean")
}
