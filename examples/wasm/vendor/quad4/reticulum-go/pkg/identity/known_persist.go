// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package identity

import (
	"encoding/hex"
	"os"
	"sync"
	"sync/atomic"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/debug"
)

var (
	// knownPersistMu guards knownPersistConfig only. knownPersistMemory and
	// knownPersistDisabled are atomics because Init/load/save run
	// concurrently with markKnownDestinationsDirty on the Remember hot path
	// whenever multiple Transport instances share this process (each one
	// calls InitKnownDestinationsPersistence independently).
	knownPersistMu       sync.Mutex
	knownPersistConfig   string
	knownPersistMemory   atomic.Bool
	knownPersistDisabled atomic.Bool
	knownPersistDirty    atomic.Bool
	knownPersistSaving   sync.Mutex
)

// knownDestRecord is a fully-decoded, package-state-independent known
// destination entry. decodeKnownDestinations produces these without
// touching the knownDestinations map, keeping the parser fuzzable and
// unit-testable in isolation.
type knownDestRecord struct {
	destHash  []byte
	publicKey []byte
	packetRaw []byte
	appData   []byte
}

// decodeKnownDestinations parses a known_destinations snapshot. Both
// hex-string map keys and raw destination-hash bin or str keys are
// accepted. Malformed entries are skipped and counted. A structurally
// invalid top-level payload that is not a msgpack map is returned as an
// error.
func decodeKnownDestinations(data []byte) (records []knownDestRecord, skipped int, err error) {
	var loaded map[string]any
	if err := msgpack.Unmarshal(data, &loaded); err != nil {
		return nil, 0, err
	}

	for hashKey, raw := range loaded {
		entry, ok := raw.([]any)
		if !ok || len(entry) < 4 {
			skipped++
			continue
		}

		publicKey, ok := entry[2].([]byte)
		if !ok || len(publicKey) != KeySize/8 {
			skipped++
			continue
		}

		destHash, ok := resolveDestHashKey(hashKey)
		if !ok {
			skipped++
			continue
		}

		var appData []byte
		if appDataRaw, ok := entry[3].([]byte); ok {
			appData = append([]byte(nil), appDataRaw...)
		}
		var packetRaw []byte
		if packetHash, ok := entry[1].([]byte); ok && len(packetHash) > 0 {
			packetRaw = append([]byte(nil), packetHash...)
		}

		records = append(records, knownDestRecord{
			destHash:  destHash,
			publicKey: append([]byte(nil), publicKey...),
			packetRaw: packetRaw,
			appData:   appData,
		})
	}

	return records, skipped, nil
}

// resolveDestHashKey accepts either a hex-encoded key (native on-disk
// format) or a raw truncated-hash-length byte string. The msgpack decoder
// folds bin and str keys into a Go string of equal length regardless of
// the original wire type.
func resolveDestHashKey(hashKey string) ([]byte, bool) {
	if decoded, err := hex.DecodeString(hashKey); err == nil && len(decoded) == TruncatedHashLength/8 {
		return decoded, true
	}
	if len(hashKey) == TruncatedHashLength/8 {
		return []byte(hashKey), true
	}
	return nil, false
}

// InitKnownDestinationsPersistence configures known-destination disk
// persistence. Pass inMemory=true to keep the table in RAM only (e.g. for
// shared-instance clients or when the operator opted out via config/env).
func InitKnownDestinationsPersistence(configPath string, inMemory bool) {
	knownPersistMu.Lock()
	defer knownPersistMu.Unlock()

	knownPersistConfig = configPath
	knownPersistMemory.Store(inMemory)
	knownPersistDisabled.Store(false)
	knownPersistDirty.Store(false)

	if configPath == "" && os.Getenv("RETICULUM_STORAGE_PATH") == "" {
		// No config path was resolved: this is either ad-hoc/library use or
		// a test harness. Never silently write into the caller's home
		// directory. Callers that want on-disk persistence must supply a

		// config path (as the real reticulum-go binary always does) or set
		// RETICULUM_STORAGE_PATH explicitly.
		knownPersistMemory.Store(true)
	}

	if knownPersistMemory.Load() {
		return
	}

	if _, err := storage.EnsureDataDir(configPath); err != nil {
		debug.Log(debug.DebugInfo, "Known destinations persistence disabled; storage unavailable", "error", err)
		knownPersistMemory.Store(true)
		knownPersistDisabled.Store(true)
		return
	}

	loadKnownDestinationsFromDisk(configPath)
}

func loadKnownDestinationsFromDisk(configPath string) {
	path, err := storage.KnownDestinationsPath(configPath)
	if err != nil {
		return
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator-controlled storage path
	if err != nil {
		if !os.IsNotExist(err) {
			debug.Log(debug.DebugInfo, "Known destinations load failed; using in-memory table", "error", err)
			knownPersistMemory.Store(true)
			knownPersistDisabled.Store(true)
		}
		return
	}
	if len(data) == 0 {
		return
	}

	records, skipped, err := decodeKnownDestinations(data)
	if err != nil {
		debug.Log(debug.DebugInfo, "Known destinations decode failed; using in-memory table", "error", err)
		knownPersistMemory.Store(true)
		knownPersistDisabled.Store(true)
		return
	}
	if skipped > 0 {
		debug.Log(debug.DebugInfo, "Skipped malformed known destination entries", "count", skipped)
	}

	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()

	count := 0
	for _, rec := range records {
		id := FromPublicKey(rec.publicKey)
		if id == nil {
			continue
		}
		canonicalKey := hex.EncodeToString(rec.destHash)
		knownDestinations[canonicalKey] = []any{
			rec.packetRaw,
			rec.destHash,
			id,
			rec.appData,
		}
		count++
	}

	if count > 0 {
		debug.Log(debug.DebugInfo, "Loaded known destinations from storage", "count", count)
	}
}

// markKnownDestinationsDirty is called from the Remember hot path. It is a
// single atomic store guarded by two already-loaded bool fields, so it adds
// no measurable overhead and never allocates.
func markKnownDestinationsDirty() {
	if knownPersistMemory.Load() || knownPersistDisabled.Load() {
		return
	}
	knownPersistDirty.Store(true)
}

// SaveKnownDestinationsSync writes known destinations unconditionally. Call
// on transport shutdown.
func SaveKnownDestinationsSync() {
	saveKnownDestinations(true)
}

// PersistKnownDestinationsIfDirty writes known destinations only when
// Remember has recorded new data since the last save. Called from periodic
// maintenance.
func PersistKnownDestinationsIfDirty() {
	saveKnownDestinations(false)
}

func saveKnownDestinations(force bool) {
	if knownPersistMemory.Load() || knownPersistDisabled.Load() {
		return
	}
	if !force && !knownPersistDirty.Load() {
		return
	}
	if !knownPersistSaving.TryLock() {
		return
	}
	defer knownPersistSaving.Unlock()

	knownDestinationsLock.RLock()
	export := make(map[string][]any, len(knownDestinations))
	for hashKey, data := range knownDestinations {
		if len(data) < 4 {
			continue
		}
		destHash, _ := data[1].([]byte)
		id, _ := data[2].(*Identity)
		appData, _ := data[3].([]byte)
		if id == nil || len(destHash) == 0 {
			continue
		}
		var packetHash []byte
		if packetBytes, ok := data[0].([]byte); ok {
			packetHash = packetBytes
		}
		key := hashKey
		if key == "" {
			key = hex.EncodeToString(destHash)
		}
		export[key] = []any{
			float64(0),
			packetHash,
			id.GetPublicKey(),
			appData,
			float64(0),
		}
	}
	knownDestinationsLock.RUnlock()

	encoded, err := msgpack.Marshal(export)
	if err != nil {
		debug.Log(debug.DebugInfo, "Known destinations marshal failed", "error", err)
		return
	}

	knownPersistMu.Lock()
	cfgPath := knownPersistConfig
	knownPersistMu.Unlock()

	path, err := storage.KnownDestinationsPath(cfgPath)
	if err != nil {
		disableKnownDestinationsPersistence(err)
		return
	}
	if err := storage.AtomicWriteFile(path, encoded, 0o600); err != nil {
		disableKnownDestinationsPersistence(err)
		return
	}
	knownPersistDirty.Store(false)
	debug.Log(debug.DebugVerbose, "Saved known destinations to storage", "count", len(export))
}

func disableKnownDestinationsPersistence(err error) {
	debug.Log(debug.DebugInfo, "Known destinations persistence disabled; continuing in-memory", "error", err)
	knownPersistMemory.Store(true)
	knownPersistDisabled.Store(true)
	knownPersistDirty.Store(false)
}
