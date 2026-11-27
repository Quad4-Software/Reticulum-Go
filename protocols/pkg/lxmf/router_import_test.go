// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
)

func copyPythonStateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", "pythonstate", "storage")
	dst := filepath.Join(root, "storage")
	if err := os.MkdirAll(filepath.Join(dst, "messagestore"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"peers"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ents, err := os.ReadDir(filepath.Join(src, "messagestore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		data, err := os.ReadFile(filepath.Join(src, "messagestore", ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, "messagestore", ent.Name()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestPythonStateImportFixture(t *testing.T) {
	storage := copyPythonStateFixture(t)
	sourcePeers := filepath.Join(storage, "peers")
	beforePeers, err := os.ReadFile(sourcePeers)
	if err != nil {
		t.Fatal(err)
	}
	sourceSum := sha256.Sum256(beforePeers)

	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(id, tr, RouterOptions{Config: DefaultConfig(), StoragePath: storage, MessagesDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		router.Stop()
		_ = tr.Close()
	}()
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}

	importStore := filepath.Join(storage, "golxmd", "messagestore")
	entries, err := os.ReadDir(importStore)
	if err != nil || len(entries) != 1 {
		t.Fatalf("imported store entries=%v err=%v", entries, err)
	}
	if router.store.Count() != 1 {
		t.Fatalf("store count=%d", router.store.Count())
	}

	peerPath := filepath.Join(storage, "golxmd", "peers", "11111111111111111111111111111111")
	if _, err := os.Stat(peerPath); err != nil {
		t.Fatalf("imported peer missing: %v", err)
	}
	router.peersMu.RLock()
	peer := router.peers["11111111111111111111111111111111"]
	router.peersMu.RUnlock()
	if peer == nil {
		t.Fatal("peer not loaded from imported python state")
	}
	if !peer.Alive || peer.Incoming != 3 || peer.RXBytes != 100 || peer.TXBytes != 200 || peer.PropagationStampCost != 16 || peer.PeeringCost != 18 {
		t.Fatalf("unexpected imported peer: %+v", peer)
	}

	afterPeers, err := os.ReadFile(sourcePeers)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(afterPeers) != sourceSum {
		t.Fatal("python peers file was modified")
	}
	sourceStoreEntries, err := os.ReadDir(filepath.Join(storage, "messagestore"))
	if err != nil || len(sourceStoreEntries) != 1 {
		t.Fatalf("python messagestore changed entries=%v err=%v", sourceStoreEntries, err)
	}

	before, err := os.ReadFile(filepath.Join(importStore, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	router.preparePythonStateImport()
	after, err := os.ReadFile(filepath.Join(importStore, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("second import rewrote imported message")
	}
	if n := len(mustReadDir(t, filepath.Join(storage, "golxmd", "peers"))); n != 1 {
		t.Fatalf("duplicate peer import count=%d", n)
	}
}

func mustReadDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
