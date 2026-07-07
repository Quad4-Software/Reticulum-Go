// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"os"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/internal/storage"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

// pathRecord is a fully-decoded, transport-independent path table entry.
// decodePathTableEntries produces these without touching Transport state,
// which keeps the parser fuzzable and unit-testable in isolation.
type pathRecord struct {
	destHash    []byte
	nextHop     []byte
	hops        uint8
	interfaceID []byte
	lastUpdated time.Time
}

type pendingPathEntry struct {
	destHash     []byte
	nextHop      []byte
	interfaceKey []byte
	hops         uint8
	lastUpdated  time.Time
}

// decodePathTableEntries parses a destination_table snapshot. It never
// panics on malformed input: individual malformed entries are skipped and
// the number of skipped entries is reported so callers/tests can assert on
// corruption handling. A structurally invalid top-level payload (not a
// msgpack array) is returned as an error.
func decodePathTableEntries(data []byte, now time.Time) (records []pathRecord, skipped int, err error) {
	var entries []any
	if err := msgpack.Unmarshal(data, &entries); err != nil {
		return nil, 0, err
	}

	truncLen := packet.TruncatedHashLength

	for _, raw := range entries {
		entry, ok := raw.([]any)
		if !ok || len(entry) < 8 {
			skipped++
			continue
		}

		destHash, ok := entry[0].([]byte)
		if !ok || len(destHash) != truncLen {
			skipped++
			continue
		}

		timestamp, ok := entry[1].(float64)
		if !ok {
			skipped++
			continue
		}

		receivedFrom, ok := entry[2].([]byte)
		if !ok {
			skipped++
			continue
		}

		hops, ok := decodeHops(entry[3])
		if !ok {
			skipped++
			continue
		}

		expires, ok := entry[4].(float64)
		if !ok {
			skipped++
			continue
		}
		if expires > 0 && now.Unix() > int64(expires) {
			continue
		}

		ifaceKey, ok := entry[6].([]byte)
		if !ok {
			skipped++
			continue
		}

		lastUpdated := time.Unix(int64(timestamp), 0)
		if timestamp <= 0 {
			lastUpdated = now
		}
		if now.Sub(lastUpdated) > time.Duration(PathRequestTTL)*time.Second {
			continue
		}

		records = append(records, pathRecord{
			destHash:    append([]byte(nil), destHash...),
			nextHop:     append([]byte(nil), receivedFrom...),
			hops:        hops,
			interfaceID: append([]byte(nil), ifaceKey...),
			lastUpdated: lastUpdated,
		})
	}

	return records, skipped, nil
}

func decodeHops(v any) (uint8, bool) {
	switch n := v.(type) {
	case uint8:
		return n, true
	case int8:
		if n < 0 {
			return 0, false
		}
		return uint8(n), true
	case uint64:
		if n > 255 {
			return 0, false
		}
		return uint8(n), true
	case int64:
		if n < 0 || n > 255 {
			return 0, false
		}
		return uint8(n), true
	case float64:
		if n < 0 || n > 255 {
			return 0, false
		}
		return uint8(n), true
	default:
		return 0, false
	}
}

func (t *Transport) initPathPersistence(cfg *common.ReticulumConfig) {
	if cfg != nil {
		cfg.ApplyPersistenceEnv()
	}
	t.pathPersistMemory.Store(cfg != nil && cfg.InMemoryPathTable)
	if cfg != nil && cfg.ConnectedToSharedInstance {
		t.pathPersistMemory.Store(true)
	}
	if configPath(cfg) == "" && os.Getenv("RETICULUM_STORAGE_PATH") == "" {
		// No config path was resolved: this is either ad-hoc/library use or
		// a test harness. Never silently write into the caller's home
		// directory. Callers that want on-disk persistence must supply

		// cfg.ConfigPath (as the real reticulum-go binary always does) or
		// set RETICULUM_STORAGE_PATH explicitly.
		t.pathPersistMemory.Store(true)
	}
	if t.pathPersistMemory.Load() {
		return
	}

	dir, err := storage.EnsureDataDir(configPath(cfg))
	if err != nil {
		debug.Log(debug.DebugInfo, "Path table persistence disabled; storage unavailable", "error", err)
		t.pathPersistMemory.Store(true)
		t.pathPersistDisabled.Store(true)
		return
	}
	t.pathPersistDir = dir
	t.pendingPathEntries = make([]pendingPathEntry, 0)
	t.loadPathTableFromDisk()
}

func configPath(cfg *common.ReticulumConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.ConfigPath
}

// interfacePersistKey derives a stable identifier for name that survives
// process restarts. Real Reticulum uses the interface object's own hash.
// Go interfaces are recreated from config on every start, so hashing the
// configured name gives the same round-trip property.
func interfacePersistKey(name string) []byte {
	return identity.TruncatedHash([]byte(name))
}

func (t *Transport) findInterfaceForPersistKey(key []byte) (common.NetworkInterface, string) {
	if len(key) == 0 {
		return nil, ""
	}
	for name, iface := range t.interfaces {
		if iface == nil {
			continue
		}
		if bytesEqual(key, interfacePersistKey(name)) {
			return iface, name
		}
	}
	return nil, ""
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// loadPathTableFromDisk reads and applies the destination_table snapshot.
// Entries whose interface cannot be resolved yet (not registered before
// this call) are held in pendingPathEntries and activated later by
// activatePendingPathsForInterface.
func (t *Transport) loadPathTableFromDisk() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	path, err := storage.DestinationTablePath(configPath(t.config))
	if err != nil {
		debug.Log(debug.DebugInfo, "Skipping path table load", "error", err)
		return
	}
	data, err := os.ReadFile(path) // #nosec G304 -- operator-controlled storage path
	if err != nil {
		if !os.IsNotExist(err) {
			debug.Log(debug.DebugInfo, "Path table load failed; using in-memory table", "error", err)
			t.pathPersistMemory.Store(true)
			t.pathPersistDisabled.Store(true)
		}
		return
	}
	if len(data) == 0 {
		return
	}

	records, skipped, err := decodePathTableEntries(data, time.Now())
	if err != nil {
		debug.Log(debug.DebugInfo, "Path table decode failed; using in-memory table", "error", err)
		t.pathPersistMemory.Store(true)
		t.pathPersistDisabled.Store(true)
		return
	}
	if skipped > 0 {
		debug.Log(debug.DebugInfo, "Skipped malformed path table entries", "count", skipped)
	}

	loaded := 0
	for _, rec := range records {
		iface, _ := t.findInterfaceForPersistKey(rec.interfaceID)
		if iface == nil {
			t.pendingPathEntries = append(t.pendingPathEntries, pendingPathEntry{
				destHash:     rec.destHash,
				nextHop:      rec.nextHop,
				interfaceKey: rec.interfaceID,
				hops:         rec.hops,
				lastUpdated:  rec.lastUpdated,
			})
			continue
		}

		key := pathMapKey(rec.destHash)
		t.paths[key] = &common.Path{
			NextHop:     rec.nextHop,
			Interface:   iface,
			Hops:        rec.hops,
			HopCount:    rec.hops,
			LastUpdated: rec.lastUpdated,
		}
		t.pathStates[key] = StateUnknown
		loaded++
	}

	if loaded > 0 {
		debug.Log(debug.DebugInfo, "Loaded path table entries from storage", "count", loaded)
	}
}

// activatePendingPathsForInterface must be called with t.mutex held.
func (t *Transport) activatePendingPathsForInterface(name string, iface common.NetworkInterface) {
	if len(t.pendingPathEntries) == 0 || iface == nil {
		return
	}
	key := interfacePersistKey(name)
	remaining := t.pendingPathEntries[:0]
	for _, pe := range t.pendingPathEntries {
		if !bytesEqual(pe.interfaceKey, key) {
			remaining = append(remaining, pe)
			continue
		}
		destKey := pathMapKey(pe.destHash)
		t.paths[destKey] = &common.Path{
			NextHop:     pe.nextHop,
			Interface:   iface,
			Hops:        pe.hops,
			HopCount:    pe.hops,
			LastUpdated: pe.lastUpdated,
		}
		t.pathStates[destKey] = StateUnknown
	}
	t.pendingPathEntries = remaining
}

// markPathTableDirty is called from the UpdatePath / cleanup hot paths.
// It is a single atomic store guarded by two already-loaded bool fields, so
// it adds no measurable overhead and never allocates.
func (t *Transport) markPathTableDirty() {
	if t.pathPersistMemory.Load() || t.pathPersistDisabled.Load() {
		return
	}
	t.pathPersistDirty.Store(true)
}

func (t *Transport) persistPathTableIfDirty() {
	if t.pathPersistMemory.Load() || t.pathPersistDisabled.Load() || !t.pathPersistDirty.Load() {
		return
	}
	t.savePathTable(false)
}

func (t *Transport) savePathTableSync() {
	if t.pathPersistMemory.Load() || t.pathPersistDisabled.Load() {
		return
	}
	t.savePathTable(true)
}

func (t *Transport) savePathTable(force bool) {
	if !force && !t.pathPersistDirty.Load() {
		return
	}
	if !t.pathPersistSaving.TryLock() {
		return
	}
	defer t.pathPersistSaving.Unlock()

	t.mutex.RLock()
	serialised := make([]any, 0, len(t.paths))
	now := time.Now()
	expires := float64(now.Unix()) + 7*24*3600
	truncLen := packet.TruncatedHashLength

	for key, path := range t.paths {
		if path == nil || path.Interface == nil {
			continue
		}
		ifaceName := path.Interface.GetName()
		iface := t.interfaces[ifaceName]
		if iface == nil || iface != path.Interface {
			continue
		}

		destHash := append([]byte(nil), key[:truncLen]...)
		nextHop := append([]byte(nil), path.NextHop...)
		serialised = append(serialised, []any{
			destHash,
			float64(path.LastUpdated.Unix()),
			nextHop,
			path.HopCount,
			expires,
			[]any{},
			interfacePersistKey(ifaceName),
			[]byte{},
		})
	}
	t.mutex.RUnlock()

	data, err := msgpack.Marshal(serialised)
	if err != nil {
		debug.Log(debug.DebugInfo, "Path table marshal failed", "error", err)
		return
	}

	path, err := storage.DestinationTablePath(configPath(t.config))
	if err != nil {
		t.disablePathPersistence(err)
		return
	}
	if err := storage.AtomicWriteFile(path, data, 0o600); err != nil {
		t.disablePathPersistence(err)
		return
	}
	t.pathPersistDirty.Store(false)
	debug.Log(debug.DebugVerbose, "Saved path table to storage", "entries", len(serialised))
}

func (t *Transport) disablePathPersistence(err error) {
	debug.Log(debug.DebugInfo, "Path table persistence disabled; continuing in-memory", "error", err)
	t.pathPersistMemory.Store(true)
	t.pathPersistDisabled.Store(true)
	t.pathPersistDirty.Store(false)
}
