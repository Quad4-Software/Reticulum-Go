// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build !js && !wasip1 && !tinygo && ((darwin && (amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (illumos && amd64) || (linux && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (netbsd && amd64) || (openbsd && (amd64 || arm64)) || windows)

package sqlitestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnownDestinationsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenKnownDestinations(filepath.Join(dir, "known_destinations.db"), 16, false)
	if err != nil {
		t.Fatal(err)
	}
	lr := 12.5
	e := &KnownDestEntry{
		DestinationHash: []byte("0123456789abcdef"),
		LastHeard:       1700000000.5,
		PacketHash:      []byte("p"),
		PublicKey:       append([]byte(nil), make([]byte, 64)...),
		AppData:         []byte("app"),
		Used:            2,
		Stamp:           []byte("12345678901234"),
		StampValue:      10.5,
		LRCost:          &lr,
	}
	s.MarkDirty(e)
	s.MarkRatchetDirty(e.DestinationHash, make([]byte, 32))
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s.Checkpoint()
	s.Close()

	ro, err := OpenKnownDestinations(filepath.Join(dir, "known_destinations.db"), 16, true)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	defer ro.Close()
	rows, err := ro.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	r := rows[0]
	if r.StampValue != 10.5 || r.LRCost == nil || *r.LRCost != 12.5 || string(r.Stamp) != "12345678901234" {
		t.Fatalf("row %+v", r)
	}
	rat, err := ro.FetchRatchet(e.DestinationHash)
	if err != nil || len(rat) != 32 {
		t.Fatalf("ratchet %v %d", err, len(rat))
	}
}

func TestAnnounceCacheOps(t *testing.T) {
	dir := t.TempDir()
	c, err := OpenAnnounceCache(filepath.Join(dir, "announces.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Store([]byte("hash1"), []byte("raw1"), "iface0")
	if raw, ifn, ok := c.Fetch([]byte("hash1")); !ok || string(raw) != "raw1" || ifn != "iface0" {
		t.Fatal("pending fetch failed")
	}
	if n, err := c.Flush(); err != nil || n != 1 {
		t.Fatalf("flush %d %v", n, err)
	}
	if c.PendingCount() != 0 {
		t.Fatal("pending not cleared")
	}
	if raw, _, ok := c.Fetch([]byte("hash1")); !ok || string(raw) != "raw1" {
		t.Fatal("db fetch failed")
	}
	if n, err := c.Clean(nil); err != nil || n != 1 {
		t.Fatalf("clean %d %v", n, err)
	}
	if _, _, ok := c.Fetch([]byte("hash1")); ok {
		t.Fatal("clean did not remove")
	}
}

func TestPythonRoundTrip(t *testing.T) {
	python := os.Getenv("PYTHON_INTEROP")
	if python == "" {
		t.Skip("set PYTHON_INTEROP")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "known_destinations.db")

	// Go writes; Python reads read-only.
	s, err := OpenKnownDestinations(dbPath, 16, false)
	if err != nil {
		t.Fatal(err)
	}
	pub := make([]byte, 64)
	for i := range pub {
		pub[i] = byte(i)
	}
	lr := 9.5
	s.MarkDirty(&KnownDestEntry{
		DestinationHash: make([]byte, 16), LastHeard: 1.5,
		PacketHash: []byte("pk"), PublicKey: pub, AppData: []byte("d"),
		Used: 1, Stamp: []byte("12345678901234"), StampValue: 11.25, LRCost: &lr,
	})
	s.MarkRatchetDirty(make([]byte, 16), make([]byte, 32))
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s.Checkpoint()
	s.Close()

	out, err := runPython(t, python, `
import sys
sys.path.insert(0, "/mnt/projects/Reticulum-Go/reticulum-ref")
import RNS
from RNS.Storage import KnownDestinationsStore
s = KnownDestinationsStore(sys.argv[1], hash_width=16, read_only=True)
rows = dict(s.load_all())
assert len(rows) == 1, rows
h, e = next(iter(rows.items()))
assert len(e) == 8 and abs(e[6]-11.25) < 1e-9 and e[7] == 9.5, e
r = s.fetch_ratchet(h)
assert r[0] == bytes(32), r
print("OK")
`, dbPath)
	if err != nil {
		t.Fatalf("python read of Go DB: %v\n%s", err, out)
	}

	// Python writes; Go reads.
	out, err = runPython(t, python, `
import sys, time
sys.path.insert(0, "/mnt/projects/Reticulum-Go/reticulum-ref")
import RNS
from RNS.Storage import KnownDestinationsStore
s = KnownDestinationsStore(sys.argv[1], hash_width=16)
dh = bytes(16)
pub = bytes(range(64))
s._dirty_upsert.add(dh)
entry = [time.time(), b"pkt", pub, b"data", 2.0, b"12345678901234", 10.5, 7.5]
import hashlib
row = s._KnownDestinationsStore__entry_row(dh, entry)
s._conn.execute("INSERT OR REPLACE INTO known_destinations VALUES (?,?,?,?,?,?,?,?,?,?)", row)
s._conn.commit()
s._conn.execute("INSERT OR REPLACE INTO ratchets VALUES (?,?,?)", (dh, bytes(32), time.time()))
s._conn.commit()
s.close()
print("OK")
`, dbPath)
	if err != nil {
		t.Fatalf("python write: %v\n%s", err, out)
	}
	g, err := OpenKnownDestinations(dbPath, 16, true)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	rows, err := g.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LRCost == nil || *rows[0].LRCost != 7.5 {
		t.Fatalf("rows %+v", rows)
	}
}
