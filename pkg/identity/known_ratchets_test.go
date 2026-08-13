// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

func resetKnownRatchets(t *testing.T) {
	t.Helper()
	ratchetPersistLock.Lock()
	knownRatchets = make(map[destMapKey]knownRatchetEntry)
	ratchetPersistLock.Unlock()
	t.Cleanup(func() {
		ratchetPersistLock.Lock()
		knownRatchets = make(map[destMapKey]knownRatchetEntry)
		ratchetPersistLock.Unlock()
	})
}

func TestRememberGetRatchet(t *testing.T) {
	resetKnownRatchets(t)
	InitKnownDestinationsPersistence("", true)

	destHash := make([]byte, TruncatedHashLength/8)
	for i := range destHash {
		destHash[i] = byte(i + 1)
	}
	pub := make([]byte, RatchetSize/8)
	for i := range pub {
		pub[i] = byte(0xA0 + i)
	}

	if got := GetRatchet(destHash); got != nil {
		t.Fatal("expected no ratchet before remember")
	}

	RememberRatchet(destHash, pub)
	got := GetRatchet(destHash)
	if !bytes.Equal(got, pub) {
		t.Fatalf("GetRatchet = %x, want %x", got, pub)
	}

	id := CurrentRatchetID(destHash)
	wantID := cryptographyHash10(pub)
	if !bytes.Equal(id, wantID) {
		t.Fatalf("CurrentRatchetID = %x, want %x", id, wantID)
	}

	pub[0] ^= 0xFF
	again := GetRatchet(destHash)
	if bytes.Equal(again, pub) {
		t.Fatal("GetRatchet aliased caller buffer")
	}
}

func cryptographyHash10(pub []byte) []byte {
	id, err := New()
	if err != nil {
		return nil
	}
	return id.GetRatchetID(pub)
}

func TestRememberRatchetRejectsWrongSize(t *testing.T) {
	resetKnownRatchets(t)
	InitKnownDestinationsPersistence("", true)

	destHash := bytes.Repeat([]byte{0x11}, TruncatedHashLength/8)
	RememberRatchet(destHash, []byte("short"))
	if GetRatchet(destHash) != nil {
		t.Fatal("short ratchet should be ignored")
	}
}

func TestGetRatchetPersistRoundtrip(t *testing.T) {
	resetKnownRatchets(t)
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config")
	if err := os.WriteFile(cfg, []byte("[reticulum]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	InitKnownDestinationsPersistence(cfg, false)
	t.Cleanup(func() { InitKnownDestinationsPersistence("", true) })

	destHash := bytes.Repeat([]byte{0x22}, TruncatedHashLength/8)
	pub := bytes.Repeat([]byte{0x33}, RatchetSize/8)
	RememberRatchet(destHash, pub)

	path := filepath.Join(tmp, "storage", "ratchets", hex.EncodeToString(destHash))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected persisted ratchet file: %v", err)
	}
	var parsed knownRatchetFile
	if err := msgpack.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("python-compatible msgpack: %v", err)
	}
	if !bytes.Equal(parsed.Ratchet, pub) {
		t.Fatalf("persisted ratchet = %x, want %x", parsed.Ratchet, pub)
	}

	ratchetPersistLock.Lock()
	knownRatchets = make(map[destMapKey]knownRatchetEntry)
	ratchetPersistLock.Unlock()

	got := GetRatchet(destHash)
	if !bytes.Equal(got, pub) {
		t.Fatalf("loaded ratchet = %x, want %x", got, pub)
	}
}

func TestGetRatchetExpired(t *testing.T) {
	resetKnownRatchets(t)
	InitKnownDestinationsPersistence("", true)

	destHash := bytes.Repeat([]byte{0x44}, TruncatedHashLength/8)
	pub := bytes.Repeat([]byte{0x55}, RatchetSize/8)
	key := knownDestKey(destHash)
	ratchetPersistLock.Lock()
	knownRatchets[key] = knownRatchetEntry{
		key:      append([]byte(nil), pub...),
		received: time.Now().Unix() - RatchetExpiry - 10,
	}
	ratchetPersistLock.Unlock()

	if GetRatchet(destHash) != nil {
		t.Fatal("expired ratchet should not be returned")
	}
}

func TestCleanKnownRatchetsSkipsDestPrivateFiles(t *testing.T) {
	resetKnownRatchets(t)
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config")
	if err := os.WriteFile(cfg, []byte("[reticulum]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	InitKnownDestinationsPersistence(cfg, false)
	t.Cleanup(func() { InitKnownDestinationsPersistence("", true) })

	dir := filepath.Join(tmp, "storage", "ratchets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := hex.EncodeToString(bytes.Repeat([]byte{0x66}, TruncatedHashLength/8))
	privateFmt, err := msgpack.Marshal(map[string][]byte{
		"signature": bytes.Repeat([]byte{0x01}, 64),
		"ratchets":  []byte{0x02},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, privateFmt, 0o600); err != nil {
		t.Fatal(err)
	}

	CleanKnownRatchets()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dest-private ratchet file was removed: %v", err)
	}
}

func TestCopyRatchetAllocBudget(t *testing.T) {
	resetKnownRatchets(t)
	InitKnownDestinationsPersistence("", true)
	destHash := bytes.Repeat([]byte{0x11}, TruncatedHashLength/8)
	pub := bytes.Repeat([]byte{0x22}, RatchetSize/8)
	RememberRatchet(destHash, pub)

	var buf [32]byte
	allocs := testing.AllocsPerRun(1000, func() {
		if n := CopyRatchet(destHash, buf[:]); n != 32 {
			t.Fatalf("CopyRatchet n=%d", n)
		}
	})
	if allocs != 0 {
		t.Fatalf("CopyRatchet allocs=%.1f want 0", allocs)
	}
}
