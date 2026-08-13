// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
)

type knownRatchetEntry struct {
	key      []byte
	received int64
}

type knownRatchetFile struct {
	Ratchet  []byte  `msgpack:"ratchet"`
	Received float64 `msgpack:"received"`
}

// RatchetPublicBytes returns the X25519 public key for a 32-byte ratchet private key.
func RatchetPublicBytes(private []byte) ([]byte, error) {
	if len(private) != RatchetSize/8 {
		return nil, errors.New("invalid ratchet key size")
	}
	return cryptography.PublicKeyFromPrivate(private)
}

// RememberRatchet stores the announced ratchet public key for destHash.
// Matches Python Identity._remember_ratchet: skip persist when the key is
// already known, and skip disk writes for in-memory / shared-instance mode.
func RememberRatchet(destHash, ratchet []byte) {
	if len(destHash) == 0 || len(ratchet) != RatchetSize/8 {
		return
	}

	key := knownDestKey(destHash)
	ratchetPersistLock.Lock()
	existing, ok := knownRatchets[key]
	if ok && bytes.Equal(existing.key, ratchet) {
		ratchetPersistLock.Unlock()
		return
	}

	knownRatchets[key] = knownRatchetEntry{
		key:      append([]byte(nil), ratchet...),
		received: time.Now().Unix(),
	}
	evictKnownRatchetsLocked(key)
	entry := knownRatchets[key]
	ratchetPersistLock.Unlock()

	debug.Log(debug.DebugTrace, "Remembering ratchet",
		"ratchet_id", hex.EncodeToString(TruncatedHash(ratchet)[:NameHashLength/8]),
		"destination", hex.EncodeToString(destHash))

	if knownPersistMemory.Load() || knownPersistDisabled.Load() {
		return
	}
	persistKnownRatchet(destHash, entry)
}

// GetRatchet returns the announced ratchet public key for destHash, or nil.
// Matches Python Identity.get_ratchet: memory first, then storage/ratchets/{hex}.
func GetRatchet(destHash []byte) []byte {
	if len(destHash) == 0 {
		return nil
	}
	key := knownDestKey(destHash)
	now := time.Now().Unix()

	ratchetPersistLock.Lock()
	if e, ok := knownRatchets[key]; ok {
		if now < e.received+RatchetExpiry && len(e.key) == RatchetSize/8 {
			out := append([]byte(nil), e.key...)
			ratchetPersistLock.Unlock()
			return out
		}
		delete(knownRatchets, key)
		ratchetPersistLock.Unlock()
		return nil
	}
	ratchetPersistLock.Unlock()

	if knownPersistMemory.Load() || knownPersistDisabled.Load() {
		return nil
	}

	loaded := loadKnownRatchetFromDisk(destHash)
	if loaded == nil {
		debug.Log(debug.DebugTrace, "Could not load ratchet", "destination", hex.EncodeToString(destHash))
		return nil
	}
	return loaded
}

// CurrentRatchetID returns the 10-byte ratchet ID for destHash, or nil.
func CurrentRatchetID(destHash []byte) []byte {
	ratchet := GetRatchet(destHash)
	if len(ratchet) == 0 {
		return nil
	}
	h := cryptography.Hash(ratchet)
	return h[:NameHashLength/8]
}

// CleanKnownRatchets drops expired in-memory entries and removes expired,
// corrupted, or orphaned on-disk known-peer ratchet files. Destination-private
// ratchet files (signed lists) in the same directory are left alone.
func CleanKnownRatchets() {
	now := time.Now().Unix()

	ratchetPersistLock.Lock()
	for k, e := range knownRatchets {
		if now >= e.received+RatchetExpiry {
			delete(knownRatchets, k)
		}
	}
	ratchetPersistLock.Unlock()

	if knownPersistMemory.Load() || knownPersistDisabled.Load() {
		return
	}

	dir, err := knownRatchetsDir()
	if err != nil || dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if len(name) != TruncatedHashLength/4 {
			continue
		}
		destHash, err := hex.DecodeString(name)
		if err != nil || len(destHash) != TruncatedHashLength/8 {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) // #nosec G304 -- operator storage path
		if err != nil {
			continue
		}
		var parsed map[string]any
		if err := msgpack.Unmarshal(data, &parsed); err != nil {
			continue
		}
		if _, hasSig := parsed["signature"]; hasSig {
			continue
		}
		expired := false
		if rec, ok := parsed["received"]; ok {
			received := asFloat64(rec)
			if float64(now) > received+float64(RatchetExpiry) {
				expired = true
			}
		} else {
			expired = true
		}

		unknown := false
		knownDestinationsLock.RLock()
		_, exists := knownDestinations[knownDestKey(destHash)]
		knownDestinationsLock.RUnlock()
		if !exists {
			unknown = true
		}

		if expired || unknown {
			_ = os.Remove(path)
		}
	}
}

func persistKnownRatchet(destHash []byte, entry knownRatchetEntry) {
	dir, err := knownRatchetsDir()
	if err != nil || dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		debug.Log(debug.DebugError, "Could not persist ratchet", "error", err)
		return
	}

	payload := knownRatchetFile{
		Ratchet:  append([]byte(nil), entry.key...),
		Received: float64(entry.received),
	}
	packed, err := msgpack.Marshal(payload)
	if err != nil {
		debug.Log(debug.DebugError, "Could not persist ratchet", "error", err)
		return
	}

	hexhash := hex.EncodeToString(destHash)
	outPath := filepath.Join(dir, hexhash+".out")
	finalPath := filepath.Join(dir, hexhash)
	if err := os.WriteFile(outPath, packed, 0o600); err != nil { // #nosec G304 -- operator storage path
		debug.Log(debug.DebugError, "Could not persist ratchet", "error", err)
		return
	}
	if err := os.Rename(outPath, finalPath); err != nil {
		_ = os.Remove(outPath)
		debug.Log(debug.DebugError, "Could not persist ratchet", "error", err)
	}
}

func loadKnownRatchetFromDisk(destHash []byte) []byte {
	dir, err := knownRatchetsDir()
	if err != nil || dir == "" {
		return nil
	}
	path := filepath.Join(dir, hex.EncodeToString(destHash))
	data, err := os.ReadFile(path) // #nosec G304 -- operator storage path
	if err != nil {
		return nil
	}

	var parsed knownRatchetFile
	if err := msgpack.Unmarshal(data, &parsed); err != nil {
		debug.Log(debug.DebugError, "An error occurred while loading ratchet data", "error", err)
		return nil
	}
	if time.Now().Unix() >= int64(parsed.Received)+RatchetExpiry || len(parsed.Ratchet) != RatchetSize/8 {
		return nil
	}

	key := knownDestKey(destHash)
	ratchetPersistLock.Lock()
	knownRatchets[key] = knownRatchetEntry{
		key:      append([]byte(nil), parsed.Ratchet...),
		received: int64(parsed.Received),
	}
	evictKnownRatchetsLocked(key)
	out := append([]byte(nil), parsed.Ratchet...)
	ratchetPersistLock.Unlock()
	return out
}

func knownRatchetsDir() (string, error) {
	knownPersistMu.Lock()
	cfg := knownPersistConfig
	knownPersistMu.Unlock()
	return storage.RatchetsDir(cfg)
}

func evictKnownRatchetsLocked(keep string) {
	if len(knownRatchets) <= MaxKnownRatchets {
		return
	}
	excess := len(knownRatchets) - MaxKnownRatchets
	for k := range knownRatchets {
		if excess <= 0 {
			return
		}
		if k == keep {
			continue
		}
		delete(knownRatchets, k)
		excess--
	}
}
