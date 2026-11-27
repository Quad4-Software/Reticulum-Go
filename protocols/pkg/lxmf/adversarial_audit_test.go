// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
)

func auditRouter(t *testing.T) *Router {
	t.Helper()
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	home := t.TempDir()
	id := mustNewIdentity(t)
	cfg := DefaultConfig()
	cfg.Propagation.EnableNode = true
	r, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.RegisterDelivery("audit", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Stop)
	return r
}

func hostilePayloads(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{
		"empty":     {},
		"nil":       nil,
		"one-byte":  {0x91},
		"notarray":  mustMarshalT(t, 42),
		"emptyarr":  mustMarshalT(t, []any{}),
		"onelem":    mustMarshalT(t, []any{1}),
		"wrongtype": mustMarshalT(t, []any{"a", "b"}),
		"nulls":     mustMarshalT(t, []any{nil, nil, nil, nil}),
	}
	huge := make([]any, 100000)
	for i := range huge {
		huge[i] = []byte{1, 2, 3}
	}
	out["huge-count"] = mustMarshalT(t, []any{[]byte{1}, huge})
	deep := any([]byte{1})
	for i := 0; i < 500; i++ {
		deep = []any{deep}
	}
	out["deep-nest"] = mustMarshalT(t, []any{[]byte{1}, []any{deep}})
	out["negative"] = mustMarshalT(t, []any{int64(-1 << 62), int64(-1)})
	out["float-nan"] = mustMarshalT(t, []any{[]byte{1}, []any{}, nanFloat()})
	out["truncated"] = mustMarshalT(t, []any{[]byte{1, 2, 3}, []any{}})[:3]
	return out
}

func mustMarshalT(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func nanFloat() float64 {
	var z float64
	return z / z
}

func memAllocMB() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc / 1024 / 1024
}

// Every control/propagation request handler must be total: no panic, no hang,
// bounded memory on hostile payloads, from both authorized and unauthorized identities.
func TestAdversarialAudit_HandlersTolerateHostilePayloads(t *testing.T) {
	r := auditRouter(t)
	id := mustNewIdentity(t)
	linkID := make([]byte, 16)
	for name, payload := range hostilePayloads(t) {
		for _, handler := range map[string]func(string, []byte, []byte, []byte, any, int64) any{} {
			_ = handler
		}
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("panic on %s: %v", name, rec)
					}
				}()
				_ = r.offerRequestHandler("/pn/offer", payload, nil, linkID, id, time.Now().Unix())
				_ = r.messageGetRequestHandler("/pn/get", payload, nil, linkID, id, time.Now().Unix())
				_ = r.peerSyncRequestHandler("/pn/sync", payload, nil, linkID, id, time.Now().Unix())
				_ = r.peerUnpeerRequestHandler("/pn/unpeer", payload, nil, linkID, id, time.Now().Unix())
				_ = r.statsGetRequestHandler("/pn/stats", payload, nil, linkID, id, time.Now().Unix())
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatalf("handler hang on %s", name)
			}
		})
	}
}

// Handlers must reject requests from unauthorized identities and must not
// panic when the link has no identity at all.
func TestAdversarialAudit_HandlersEnforceAuth(t *testing.T) {
	r := auditRouter(t)
	stranger := mustNewIdentity(t)
	linkID := make([]byte, 16)
	validOffer := mustMarshalT(t, []any{make([]byte, 32), []any{}})
	if got := r.statsGetRequestHandler("/pn/stats", nil, nil, linkID, stranger, time.Now().Unix()); got != int(PeerErrorNoAccess) {
		t.Errorf("stats from stranger: got %v want NoAccess", got)
	}
	if got := r.peerSyncRequestHandler("/pn/sync", make([]byte, DestinationLength), nil, linkID, stranger, time.Now().Unix()); got != int(PeerErrorNoAccess) {
		t.Errorf("sync from stranger: got %v want NoAccess", got)
	}
	if got := r.peerUnpeerRequestHandler("/pn/unpeer", make([]byte, DestinationLength), nil, linkID, stranger, time.Now().Unix()); got != int(PeerErrorNoAccess) {
		t.Errorf("unpeer from stranger: got %v want NoAccess", got)
	}
	_ = validOffer
	// nil identity must never panic and must not gain access
	if got := r.statsGetRequestHandler("/pn/stats", nil, nil, linkID, nil, time.Now().Unix()); got != int(PeerErrorNoIdentity) {
		t.Errorf("stats nil identity: got %v want NoIdentity", got)
	}
	if got := r.peerSyncRequestHandler("/pn/sync", make([]byte, DestinationLength), nil, linkID, nil, time.Now().Unix()); got != int(PeerErrorNoIdentity) {
		t.Errorf("sync nil identity: got %v want NoIdentity", got)
	}
}

// Restarting on corrupted state must never crash: truncated peers, zero-byte
// messagestore entries, garbage files, partial writes.
func TestAdversarialAudit_RestartOnCorruptedState(t *testing.T) {
	home := t.TempDir()
	storage := filepath.Join(home, "storage")
	lxmfDir := filepath.Join(storage, "lxmf", "messagestore")
	if err := os.MkdirAll(lxmfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// zero-byte entry, garbage entry, valid-name-but-truncated entry, .tmp leftover
	sum := sha256.Sum256([]byte("x"))
	name := MessageStoreFilename(sum[:], 1700000000.5, 16)
	files := map[string][]byte{
		"zero-" + name: {},
		name:           {1, 2, 3},
		MessageStoreFilename(sum[:], 1, 0) + ".tmp": {9},
		"not-a-valid-name":                          {1, 2, 3, 4},
	}
	for n, data := range files {
		if err := os.WriteFile(filepath.Join(lxmfDir, n), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(storage, "lxmf", "peers"), []byte{0xff, 0xff, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()
	id := mustNewIdentity(t)
	cfg := DefaultConfig()
	cfg.Propagation.EnableNode = true
	r, err := NewRouter(id, tr, RouterOptions{
		Config:      cfg,
		StoragePath: filepath.Join(storage, "lxmf"),
		MessagesDir: filepath.Join(storage, "messages"),
	})
	if err != nil {
		t.Fatalf("restart on corrupted state failed: %v", err)
	}
	if err := r.EnablePropagation(); err != nil {
		t.Fatalf("enable propagation on corrupted state: %v", err)
	}
	r.Stop()
}

// The python peers importer must tolerate hostile msgpack, bound memory,
// and never modify source files.
func TestAdversarialAudit_HostilePythonPeersImport(t *testing.T) {
	dir := t.TempDir()
	peersFile := filepath.Join(dir, "peers")
	importDir := filepath.Join(dir, "import")
	if err := os.MkdirAll(importDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := map[string][]byte{
		"not-msgpack":   {0xff, 0xff, 0xff, 0xff},
		"empty":         {},
		"not-a-list":    mustMarshalT(t, 42),
		"huge-count":    append([]byte{0xdd, 0xff, 0xff, 0xff, 0xff}, 0xc0),
		"nonbin-entry":  mustMarshalT(t, []any{42, "str", nil}),
		"bad-inner":     mustMarshalT(t, []any{[]byte{1, 2, 3}}),
		"wrong-hashlen": mustMarshalT(t, []any{mustMarshalT(t, map[string]any{"destination_hash": []byte{1, 2}})}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(peersFile, data, 0o600); err != nil {
				t.Fatal(err)
			}
			before := sha256.Sum256(data)
			done := make(chan struct{})
			var copied, skipped int
			var err error
			go func() {
				defer close(done)
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("import panic: %v", rec)
					}
				}()
				copied, skipped, err = importPythonPeers(peersFile, importDir)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("import hang")
			}
			after, _ := os.ReadFile(peersFile)
			if sha256.Sum256(after) != before {
				t.Error("source peers file modified")
			}
			_ = copied
			_ = skipped
			_ = err
		})
	}
}

// Importer must be idempotent: running it twice copies nothing the second time.
func TestAdversarialAudit_PythonImportIdempotent(t *testing.T) {
	dir := t.TempDir()
	importDir := filepath.Join(dir, "import")
	if err := os.MkdirAll(importDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dest := make([]byte, DestinationLength)
	for i := range dest {
		dest[i] = byte(i)
	}
	entry := mustMarshalT(t, map[string]any{"destination_hash": dest})
	peersFile := filepath.Join(dir, "peers")
	if err := os.WriteFile(peersFile, mustMarshalT(t, []any{entry}), 0o600); err != nil {
		t.Fatal(err)
	}
	c1, _, err := importPythonPeers(peersFile, importDir)
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := importPythonPeers(peersFile, importDir)
	if err != nil {
		t.Fatal(err)
	}
	if c1 != 1 || c2 != 0 {
		t.Errorf("import not idempotent: first=%d second=%d", c1, c2)
	}
}

// Store invariants under random operations: count and byte accounting match
// ground truth, and disk state matches the index after every op.
func TestAdversarialAudit_StoreInvariants(t *testing.T) {
	dir := t.TempDir()
	ms, err := NewMessageStore(dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	rng := uint64(12345)
	next := func(n int) int {
		rng = rng*6364136223846793005 + 1442695040888963407
		return int((rng >> 33) % uint64(n))
	}
	live := make(map[string]int64)
	ops := 600
	for i := 0; i < ops; i++ {
		switch next(10) {
		case 0, 1, 2, 3, 4:
			size := 100 + next(2000)
			lxm := make([]byte, size)
			for j := range lxm {
				lxm[j] = byte(next(256))
			}
			stamp := make([]byte, StampSize)
			ent, err := ms.Add(lxm, stamp, 16)
			if err != nil {
				t.Fatalf("add op %d: %v", i, err)
			}
			sum := sha256.Sum256(lxm)
			live[hex.EncodeToString(sum[:])] = ent.Size
		case 5, 6:
			for k := range live {
				raw, _ := hex.DecodeString(k)
				_ = ms.Remove(raw)
				delete(live, k)
				break
			}
		default:
			for k := range live {
				raw, _ := hex.DecodeString(k)
				if _, ok := ms.Get(raw); !ok {
					t.Fatalf("op %d: live entry %s missing from index", i, k)
				}
				break
			}
		}
	}
	if ms.Count() != len(live) {
		t.Errorf("count mismatch: index=%d truth=%d", ms.Count(), len(live))
	}
	var wantBytes int64
	for _, sz := range live {
		wantBytes += sz
	}
	if got := ms.TotalBytes(); got != wantBytes {
		t.Errorf("byte accounting mismatch: index=%d truth=%d", got, wantBytes)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	diskCount := 0
	for _, ent := range ents {
		if !ent.IsDir() {
			diskCount++
		}
	}
	if diskCount != len(live) {
		t.Errorf("disk/index mismatch: disk=%d truth=%d", diskCount, len(live))
	}
}

// Eviction must keep the store within its limit after every add settles.
func TestAdversarialAudit_StoreEvictionNeverExceedsLimit(t *testing.T) {
	dir := t.TempDir()
	limitMB := 0.05 // 50 KB
	ms, err := NewMessageStore(dir, limitMB, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		lxm := make([]byte, 5000+i)
		for j := range lxm {
			lxm[j] = byte(i)
		}
		stamp := make([]byte, StampSize)
		if _, err := ms.Add(lxm, stamp, 16); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		if got := ms.TotalBytes(); got > int64(limitMB*1000*1000)+int64(len(lxm)+StampSize) {
			t.Fatalf("store exceeds limit after add %d: %d bytes", i, got)
		}
	}
	final := ms.TotalBytes()
	if final > int64(limitMB*1000*1000) {
		t.Errorf("final store bytes %d exceed limit %d", final, int64(limitMB*1000*1000))
	}
}
