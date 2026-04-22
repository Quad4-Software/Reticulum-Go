// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package blackhole

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newLocal(t *testing.T) []byte {
	t.Helper()
	h := make([]byte, HashLen)
	if _, err := rand.Read(h); err != nil {
		t.Fatalf("rand: %v", err)
	}
	SetLocalIdentityHash(h)
	return h
}

func TestAddRemoveHas(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	id := bytes.Repeat([]byte{0x01}, HashLen)
	added, err := tab.Add(id, 0, "spam")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !added {
		t.Fatalf("expected first Add to return true")
	}
	if !tab.Has(id) {
		t.Fatalf("expected Has(id) to be true after Add")
	}
	again, err := tab.Add(id, 0, "spam")
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if again {
		t.Fatalf("re-add should return false")
	}
	removed, err := tab.Remove(id)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !removed {
		t.Fatalf("Remove should report true")
	}
	if tab.Has(id) {
		t.Fatalf("Has should be false after remove")
	}
}

func TestExpiry(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	now := time.Unix(1_700_000_000, 0)
	tab.SetClock(func() time.Time { return now })
	id := bytes.Repeat([]byte{0x02}, HashLen)
	if _, err := tab.Add(id, float64(now.Add(10*time.Second).Unix()), "tmp"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !tab.Has(id) {
		t.Fatalf("entry should be active")
	}
	tab.SetClock(func() time.Time { return now.Add(60 * time.Second) })
	if tab.Has(id) {
		t.Fatalf("entry should have expired")
	}
	if removed := tab.SweepExpired(); removed != 1 {
		t.Fatalf("expected SweepExpired to remove 1 entry, removed=%d", removed)
	}
}

func TestPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	local := newLocal(t)
	tab := New(dir)
	id1 := bytes.Repeat([]byte{0x03}, HashLen)
	id2 := bytes.Repeat([]byte{0x04}, HashLen)
	if _, err := tab.Add(id1, 0, ""); err != nil {
		t.Fatalf("add1: %v", err)
	}
	if _, err := tab.Add(id2, float64(time.Now().Add(time.Hour).Unix()), "later"); err != nil {
		t.Fatalf("add2: %v", err)
	}
	tab2 := New(dir)
	SetLocalIdentityHash(local)
	if err := tab2.LoadAll(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !tab2.Has(id1) || !tab2.Has(id2) {
		t.Fatalf("reloaded table missing entries")
	}
}

func TestMergeRemote(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	src := bytes.Repeat([]byte{0xaa}, HashLen)
	id := bytes.Repeat([]byte{0x05}, HashLen)
	decoded := map[string]Entry{
		string(id): {Source: src, Reason: "remote"},
	}
	if err := tab.MergeRemote(src, decoded); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !tab.Has(id) {
		t.Fatalf("expected merged entry to be present")
	}
	if _, err := os.Stat(filepath.Join(dir, encodeHex(src))); err != nil {
		t.Fatalf("source persisted file missing: %v", err)
	}
}

func TestEncodeDecodeRoundtrip(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	id := bytes.Repeat([]byte{0x09}, HashLen)
	if _, err := tab.Add(id, 1700000000.5, "abuse"); err != nil {
		t.Fatalf("add: %v", err)
	}
	packed, err := tab.EncodeForRequest()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodeBlackholeMap(packed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded[string(id)]
	if !ok {
		t.Fatalf("decoded map missing entry")
	}
	if got.Reason != "abuse" {
		t.Fatalf("reason mismatch: %q", got.Reason)
	}
	if got.Until != 1700000000.5 {
		t.Fatalf("until mismatch: %v", got.Until)
	}
}

func TestLocalEntriesNotOverwrittenByRemote(t *testing.T) {
	dir := t.TempDir()
	newLocal(t)
	tab := New(dir)
	id := bytes.Repeat([]byte{0x10}, HashLen)
	if _, err := tab.Add(id, 0, "local"); err != nil {
		t.Fatalf("add: %v", err)
	}
	src := bytes.Repeat([]byte{0xbb}, HashLen)
	if err := tab.MergeRemote(src, map[string]Entry{string(id): {Source: src, Reason: "remote"}}); err != nil {
		t.Fatalf("merge: %v", err)
	}
	entries := tab.All()
	var k [HashLen]byte
	copy(k[:], id)
	got := entries[k]
	if got.Reason != "local" {
		t.Fatalf("local reason was overwritten: %q", got.Reason)
	}
}
