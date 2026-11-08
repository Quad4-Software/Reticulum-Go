// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/internal/sqlitestore"
	"github.com/Quad4-Software/Reticulum-Go/internal/storage"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// knownStore holds the sqlite known-destinations DB when the driver is
// available and persistence is enabled; nil means use the legacy file.
var knownStore atomic.Pointer[sqlitestore.KnownDestinationsStore]

// knownDestinationsDBPath resolves storage/known_destinations.db (Python
// Storage.KnownDestinationsStore filename).
func knownDestinationsDBPath(configPath string) (string, error) {
	dir, err := storage.EnsureDataDir(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "known_destinations.db"), nil
}

// initKnownStore opens the DB and imports legacy persistence. Called from
// InitKnownDestinationsPersistence while knownPersistMu is held.
func initKnownStore(configPath string) {
	old := knownStore.Swap(nil)
	if old != nil {
		old.Close()
	}
	if !sqlitestore.Available || os.Getenv("RETICULUM_DISABLE_SQLITE_KNOWN") == "1" {
		return
	}
	dbPath, err := knownDestinationsDBPath(configPath)
	if err != nil {
		return
	}
	s, err := sqlitestore.OpenKnownDestinations(dbPath, 16, false)
	if err != nil {
		debug.Log(debug.DebugInfo, "Known destinations DB open failed, using legacy file persistence", "error", err)
		return
	}
	knownStore.Store(s)
	importLegacyKnownDestinations(configPath, s)
	importLegacyRatchets(configPath, s)
}

// importLegacyKnownDestinations reads the legacy msgpack file into the DB
// and renames it .imported (Python Storage behaviour).
func importLegacyKnownDestinations(configPath string, s *sqlitestore.KnownDestinationsStore) {
	path, err := storage.KnownDestinationsPath(configPath)
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	records, _, err := decodeKnownDestinations(data)
	if err != nil {
		debug.Log(debug.DebugInfo, "Legacy known destinations import skipped, decode failed", "error", err)
		return
	}
	now := float64(time.Now().UnixNano()) / 1e9
	for _, rec := range records {
		rem := rec.rememberedAt
		if rem <= 0 {
			rem = now
		}
		s.MarkDirty(&sqlitestore.KnownDestEntry{
			DestinationHash: rec.destHash,
			LastHeard:       rem,
			PacketHash:      rec.packetRaw,
			PublicKey:       rec.publicKey,
			AppData:         rec.appData,
			Used:            rec.lastUsed,
			Stamp:           rec.stamp,
			StampValue:      rec.stampValue,
			LRCost:          rec.lrCost,
		})
	}
	if err := s.Flush(); err != nil {
		debug.Log(debug.DebugInfo, "Legacy known destinations import failed", "error", err)
		return
	}
	_ = os.Rename(path, path+".imported")
	debug.Log(debug.DebugInfo, "Imported legacy known destinations into database", "count", len(records))
}

// importLegacyRatchets loads ratchet files into the ratchets table and
// removes the legacy files.
func importLegacyRatchets(configPath string, s *sqlitestore.KnownDestinationsStore) {
	dir, err := storage.RatchetsDir(configPath)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || len(raw) != 32 {
			continue
		}
		// Legacy filename is the destination hash hex.
		destHash := make([]byte, 0, 32)
		for i := 0; i+2 <= len(e.Name()) && len(destHash) < 32; i += 2 {
			var b byte
			if _, err := hexDecodeByte(e.Name()[i:i+2], &b); err != nil {
				break
			}
			destHash = append(destHash, b)
		}
		if len(destHash) == 0 {
			continue
		}
		s.MarkRatchetDirty(destHash, raw)
		n++
	}
	if n == 0 {
		return
	}
	if err := s.Flush(); err != nil {
		debug.Log(debug.DebugInfo, "Legacy ratchet import failed", "error", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	debug.Log(debug.DebugInfo, "Imported legacy ratchets into database", "count", n)
}

func hexDecodeByte(s string, out *byte) (int, error) {
	var v uint8
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v += uint8(c - '0')
		case c >= 'a' && c <= 'f':
			v += uint8(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			v += uint8(c - 'A' + 10)
		default:
			return 0, os.ErrInvalid
		}
	}
	*out = v
	return 1, nil
}

// loadKnownDestinationsFromDB loads the table into memory. Returns true
// when the sqlite backend handled the load.
func loadKnownDestinationsFromDB() bool {
	s := knownStore.Load()
	if s == nil {
		return false
	}
	rows, err := s.LoadAll()
	if err != nil {
		debug.Log(debug.DebugInfo, "Known destinations DB load failed, using in-memory table", "error", err)
		return true
	}
	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()
	count := 0
	for _, rec := range rows {
		id := FromPublicKey(rec.PublicKey)
		if id == nil {
			continue
		}
		canonicalKey := knownDestKey(rec.DestinationHash)
		knownDestinations[canonicalKey] = knownDestEntry{
			pkt:        rec.PacketHash,
			hash:       rec.DestinationHash,
			rawKey:     string(canonicalKey[:]),
			id:         id,
			app:        rec.AppData,
			stamp:      rec.Stamp,
			stampValue: rec.StampValue,
			lrCost:     rec.LRCost,
		}
		rem := int64(rec.LastHeard)
		if rem <= 0 {
			rem = time.Now().Unix()
		}
		setKnownDestMetaLocked(canonicalKey, rem, int64(rec.Used))
		count++
	}
	if count > 0 {
		debug.Log(debug.DebugInfo, "Loaded known destinations from database", "count", count)
	}
	return true
}

// saveKnownDestinationsDB flushes the in-memory table to sqlite. Returns
// true when the sqlite backend handled the write.
func saveKnownDestinationsDB() bool {
	s := knownStore.Load()
	if s == nil {
		return false
	}
	knownDestinationsLock.RLock()
	defer knownDestinationsLock.RUnlock()
	for hashKey, e := range knownDestinations {
		if e.id == nil || len(e.hash) == 0 {
			continue
		}
		meta := knownDestMetaByKey[hashKey]
		rem := float64(meta.rememberedAt)
		if rem == 0 {
			rem = float64(time.Now().Unix())
		}
		s.MarkDirty(&sqlitestore.KnownDestEntry{
			DestinationHash: e.hash,
			LastHeard:       rem,
			PacketHash:      e.pkt,
			PublicKey:       e.id.GetPublicKey(),
			AppData:         e.app,
			Used:            float64(meta.lastUsed),
			Stamp:           e.stamp,
			StampValue:      e.stampValue,
			LRCost:          e.lrCost,
		})
	}
	if err := s.Flush(); err != nil {
		debug.Log(debug.DebugInfo, "Known destinations DB flush failed", "error", err)
		return true
	}
	s.Checkpoint()
	return true
}
