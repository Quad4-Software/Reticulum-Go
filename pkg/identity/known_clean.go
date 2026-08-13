// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package identity

import (
	"runtime"
	"time"

	"quad4/reticulum-go/pkg/debug"
)

const (
	// UnusedDestinationLinger is how long pathless, never-used known
	// destinations are kept before cleaning (Python UNUSED_DESTINATION_LINGER).
	UnusedDestinationLinger = 6 * time.Minute

	// DestinationTimeout is the unused age after which pathless known
	// destinations are cleaned (Python DESTINATION_TIMEOUT).
	DestinationTimeout = 7 * 24 * time.Hour

	// knownCleanBatchSize is how many entries are inspected between yields.
	knownCleanBatchSize = 64
)

// knownDestMeta tracks announce and usage ages for background cleaning.
// lastUsed: 0 never used, -1 retained, >0 Unix seconds of last use.
type knownDestMeta struct {
	rememberedAt int64
	lastUsed     int64
}

var knownDestMetaByKey = make(map[string]knownDestMeta)

func setKnownDestMetaLocked(key string, rememberedAt, lastUsed int64) {
	if rememberedAt <= 0 {
		rememberedAt = time.Now().Unix()
	}
	knownDestMetaByKey[key] = knownDestMeta{
		rememberedAt: rememberedAt,
		lastUsed:     lastUsed,
	}
}

func deleteKnownDestMetaLocked(key string) {
	delete(knownDestMetaByKey, key)
}

// TouchKnownDestination records that destHash was used (encrypt/decrypt/link).
// No-op when the destination is unknown or retained. Does not dirty the
// persistence flag on every touch so recall storms do not force disk writes.
// lastUsed is still persisted on the next Remember or clean-triggered save.
func TouchKnownDestination(destHash []byte) {
	if len(destHash) == 0 {
		return
	}
	key := knownDestKey(destHash)
	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()
	meta, ok := knownDestMetaByKey[key]
	if !ok {
		if _, exists := knownDestinations[key]; !exists {
			return
		}
		meta.rememberedAt = time.Now().Unix()
	}
	if meta.lastUsed < 0 {
		return
	}
	meta.lastUsed = time.Now().Unix()
	knownDestMetaByKey[key] = meta
}

// RetainKnownDestination marks destHash so background cleaning never removes it.
func RetainKnownDestination(destHash []byte) {
	if len(destHash) == 0 {
		return
	}
	key := knownDestKey(destHash)
	knownDestinationsLock.Lock()
	defer knownDestinationsLock.Unlock()
	if _, exists := knownDestinations[key]; !exists {
		return
	}
	meta := knownDestMetaByKey[key]
	if meta.rememberedAt == 0 {
		meta.rememberedAt = time.Now().Unix()
	}
	meta.lastUsed = -1
	knownDestMetaByKey[key] = meta
	markKnownDestinationsDirty()
}

// CleanKnownDestinationsResult summarizes a cleaning pass.
type CleanKnownDestinationsResult struct {
	Total   int
	Removed int
	NoPath  int
}

// CleanKnownDestinations removes pathless known destinations that have aged
// out. hasPath should report whether transport still has a path for the hash.
// Unlike Python, never-used entries are only removed after UnusedDestinationLinger
// (Python also compares unused_for against DESTINATION_TIMEOUT*1.25 when
// last_use is 0, which can delete fresh announces immediately).
//
// Deletion re-validates retain/use/age under the write lock so a concurrent
// Retain, Touch, or Remember cannot lose to a stale clean verdict (TOCTOU).
func CleanKnownDestinations(hasPath func([]byte) bool) CleanKnownDestinationsResult {
	if hasPath == nil {
		hasPath = func([]byte) bool { return false }
	}

	knownDestinationsLock.RLock()
	keys := make([]string, 0, len(knownDestinations))
	for key := range knownDestinations {
		keys = append(keys, key)
	}
	knownDestinationsLock.RUnlock()

	now := time.Now()
	nowUnix := now.Unix()
	stale := make([]string, 0)
	result := CleanKnownDestinationsResult{Total: len(keys)}

	for i, key := range keys {
		if i > 0 && i%knownCleanBatchSize == 0 {
			runtime.Gosched()
		}

		destHash, ok := resolveDestHashKey(key)
		if !ok {
			continue
		}
		pathPresent := hasPath(destHash)
		if !pathPresent {
			result.NoPath++
		}

		knownDestinationsLock.RLock()
		meta, metaOK := knownDestMetaByKey[key]
		_, exists := knownDestinations[key]
		knownDestinationsLock.RUnlock()
		if !exists {
			continue
		}
		if !metaOK {
			meta = knownDestMeta{rememberedAt: nowUnix, lastUsed: 0}
		}
		if meta.lastUsed < 0 {
			continue
		}
		if pathPresent {
			continue
		}

		rememberedAt := time.Unix(meta.rememberedAt, 0)
		if meta.lastUsed == 0 {
			if now.Sub(rememberedAt) > UnusedDestinationLinger {
				stale = append(stale, key)
			}
			continue
		}
		unusedFor := nowUnix - meta.lastUsed
		if unusedFor > int64(DestinationTimeout*5/4/time.Second) {
			stale = append(stale, key)
		}
	}

	if len(stale) == 0 {
		CleanKnownRatchets()
		return result
	}

	knownDestinationsLock.Lock()
	for _, key := range stale {
		if !stillStaleForCleanLocked(key, hasPath, now, nowUnix) {
			continue
		}
		delete(knownDestinations, key)
		deleteKnownDestMetaLocked(key)
		result.Removed++
	}
	knownDestinationsLock.Unlock()

	if result.Removed > 0 {
		markKnownDestinationsDirty()
		debug.Log(debug.DebugVerbose, "Cleaned known destinations",
			"total", result.Total, "removed", result.Removed, "no_path", result.NoPath)
	}
	CleanKnownRatchets()
	return result
}

// stillStaleForCleanLocked re-checks eligibility under the write lock.
func stillStaleForCleanLocked(key string, hasPath func([]byte) bool, now time.Time, nowUnix int64) bool {
	meta, metaOK := knownDestMetaByKey[key]
	if _, exists := knownDestinations[key]; !exists {
		return false
	}
	if !metaOK {
		return false
	}
	if meta.lastUsed < 0 {
		return false
	}
	destHash, ok := resolveDestHashKey(key)
	if !ok {
		return false
	}
	if hasPath(destHash) {
		return false
	}
	if meta.lastUsed == 0 {
		return now.Sub(time.Unix(meta.rememberedAt, 0)) > UnusedDestinationLinger
	}
	return nowUnix-meta.lastUsed > int64(DestinationTimeout*5/4/time.Second)
}
