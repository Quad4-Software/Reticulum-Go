// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
)

func fuzzRouter(tb testing.TB) *Router {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		tb.Fatal(err)
	}
	home := tb.TempDir()
	id := mustNewIdentity(tb)
	cfg := DefaultConfig()
	cfg.Propagation.EnableNode = true
	cfg.Propagation.PeeringCost = 0
	r, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := r.RegisterDelivery("fuzz", nil); err != nil {
		tb.Fatal(err)
	}
	if err := r.EnablePropagation(); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		r.Stop()
		_ = tr.Close()
	})
	return r
}

func FuzzPropagationControlPayloads(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x92, 0xc0, 0x90})
	f.Add([]byte{0xdd, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x91})
	f.Add([]byte{0x92, 0xc4, 0x20, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 0x91, 0xc4, 0x20, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32})
	var once sync.Once
	var r *Router
	var ident *identity.Identity
	f.Fuzz(func(t *testing.T, data []byte) {
		once.Do(func() {
			r = fuzzRouter(t)
			ident = r.identity
		})
		linkID := make([]byte, 16)
		now := time.Now().Unix()
		_ = r.offerRequestHandler("/pn/offer", data, nil, linkID, ident, now)
		_ = r.messageGetRequestHandler("/pn/get", data, nil, linkID, ident, now)
		_ = r.peerSyncRequestHandler("/pn/sync", data, nil, linkID, ident, now)
		_ = r.peerUnpeerRequestHandler("/pn/unpeer", data, nil, linkID, ident, now)
		_ = r.statsGetRequestHandler("/pn/stats", data, nil, linkID, ident, now)
	})
}

func FuzzPythonPeersImport(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x90})
	f.Add([]byte{0xdd, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x91, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip("input too large for file round-trip")
		}
		dir := t.TempDir()
		peersFile := filepath.Join(dir, "peers")
		importDir := filepath.Join(dir, "import")
		if err := os.MkdirAll(importDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(peersFile, data, 0o600); err != nil {
			t.Fatal(err)
		}
		before := sha256.Sum256(data)
		_, _, _ = importPythonPeers(peersFile, importDir)
		after, err := os.ReadFile(peersFile)
		if err != nil {
			t.Fatal(err)
		}
		if sha256.Sum256(after) != before {
			t.Fatal("source peers file modified by import")
		}
	})
}
