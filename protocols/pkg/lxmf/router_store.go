// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const messageExpirySeconds = 30 * 24 * 60 * 60

// PropagationEntry is one stored propagation message indexed by transient ID.
type PropagationEntry struct {
	DestinationHash []byte
	FilePath        string
	ReceivedAt      float64
	Size            int64
	HandledPeers    []string
	UnhandledPeers  []string
	StampValue      int64
}

// MessageStore indexes propagation entries and enforces storage limits on disk.
type MessageStore struct {
	mu              sync.RWMutex
	dir             string
	entries         map[string]*PropagationEntry
	limitBytes      int64
	prioritisedDest map[destID]struct{}
}

// NewMessageStore loads existing files from dir and applies limitBytes (0 means unlimited).
func NewMessageStore(dir string, limitMB float64, prioritised [][]byte) (*MessageStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("lxmf: message store dir: %w", err)
	}
	ms := &MessageStore{
		dir:             dir,
		entries:         make(map[string]*PropagationEntry),
		prioritisedDest: make(map[destID]struct{}),
	}
	if limitMB > 0 {
		ms.limitBytes = int64(limitMB * 1000 * 1000)
	}
	for _, h := range prioritised {
		if key, ok := destIDFrom(h); ok {
			ms.prioritisedDest[key] = struct{}{}
		}
	}
	if err := ms.load(); err != nil {
		return nil, err
	}
	return ms, nil
}

func (ms *MessageStore) load() error {
	ents, err := os.ReadDir(ms.dir)
	if err != nil {
		return err
	}
	for _, ent := range ents {
		if ent.IsDir() || strings.HasSuffix(ent.Name(), ".tmp") {
			continue
		}
		tid, receivedAt, stampValue, err := ParseMessageStoreFilename(ent.Name())
		if err != nil {
			Warning("skipping invalid message store file", "name", ent.Name(), "error", err)
			continue
		}
		path := filepath.Join(ms.dir, ent.Name())
		data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path under store dir
		if err != nil {
			Warning("skipping unreadable message store file", "path", path, "error", err)
			continue
		}
		if len(data) < DestinationLength {
			continue
		}
		lxmLen := len(data)
		if lxmLen > StampSize {
			lxmLen -= StampSize
		}
		lxmData := data[:lxmLen]
		key := hex.EncodeToString(tid)
		ms.entries[key] = &PropagationEntry{
			DestinationHash: append([]byte(nil), lxmData[:DestinationLength]...),
			FilePath:        path,
			ReceivedAt:      receivedAt,
			Size:            int64(len(data)),
			StampValue:      stampValue,
		}
	}
	return nil
}

// Add writes stamped propagation data and indexes it by transient ID.
func (ms *MessageStore) Add(lxmData, stampData []byte, stampValue int64) (*PropagationEntry, error) {
	if len(lxmData) < DestinationLength {
		return nil, errors.New("lxmf: propagation data too short")
	}
	sum := sha256.Sum256(lxmData)
	key := hex.EncodeToString(sum[:])
	received := float64(time.Now().UnixNano()) / 1e9

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, ok := ms.entries[key]; ok {
		return ms.entries[key], nil
	}

	stamped := append(append([]byte(nil), lxmData...), stampData...)
	name := MessageStoreFilename(sum[:], received, stampValue)
	path := filepath.Join(ms.dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, stamped, 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}

	entry := &PropagationEntry{
		DestinationHash: append([]byte(nil), lxmData[:DestinationLength]...),
		FilePath:        path,
		ReceivedAt:      received,
		Size:            int64(len(stamped)),
		StampValue:      stampValue,
	}
	ms.entries[key] = entry
	ms.enforceLimitLocked()
	return entry, nil
}

// Get returns a copy of the entry for transientID hex key or raw bytes.
func (ms *MessageStore) Get(transientID []byte) (*PropagationEntry, bool) {
	key := hex.EncodeToString(transientID)
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	ent, ok := ms.entries[key]
	if !ok {
		return nil, false
	}
	return clonePropagationEntry(ent), true
}

// Remove deletes one entry from disk and memory.
func (ms *MessageStore) Remove(transientID []byte) error {
	key := hex.EncodeToString(transientID)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ent, ok := ms.entries[key]
	if !ok {
		return nil
	}
	delete(ms.entries, key)
	if ent.FilePath != "" {
		_ = os.Remove(ent.FilePath)
	}
	return nil
}

// PurgeForDestination removes entries matching destinationHash from have list processing.
func (ms *MessageStore) Purge(transientIDs [][]byte, destinationHash []byte) int {
	destKey := append([]byte(nil), destinationHash...)
	removed := 0
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, tid := range transientIDs {
		key := hex.EncodeToString(tid)
		ent, ok := ms.entries[key]
		if !ok {
			continue
		}
		if !bytesEqual(ent.DestinationHash, destKey) {
			continue
		}
		delete(ms.entries, key)
		if ent.FilePath != "" {
			_ = os.Remove(ent.FilePath)
		}
		removed++
	}
	return removed
}

// ListForDestination returns transient IDs for destination sorted by size ascending.
func (ms *MessageStore) ListForDestination(destinationHash []byte) [][]byte {
	type item struct {
		id   []byte
		size int64
	}
	ms.mu.RLock()
	items := make([]item, 0)
	for key, ent := range ms.entries {
		if !bytesEqual(ent.DestinationHash, destinationHash) {
			continue
		}
		tid, err := hex.DecodeString(key)
		if err != nil {
			continue
		}
		items = append(items, item{id: tid, size: ent.Size})
	}
	ms.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool { return items[i].size < items[j].size })
	out := make([][]byte, 0, len(items))
	for _, it := range items {
		out = append(out, append([]byte(nil), it.id...))
	}
	return out
}

// ReadLXMData reads stored LXMF bytes without the trailing stamp.
func (ms *MessageStore) ReadLXMData(transientID []byte) ([]byte, error) {
	ent, ok := ms.Get(transientID)
	if !ok {
		return nil, fmt.Errorf("lxmf: message not found")
	}
	data, err := os.ReadFile(filepath.Clean(ent.FilePath)) // #nosec G304 -- path from store index
	if err != nil {
		return nil, err
	}
	if len(data) > StampSize {
		data = data[:len(data)-StampSize]
	}
	return data, nil
}

// Count returns the number of indexed entries.
func (ms *MessageStore) Count() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.entries)
}

// TotalBytes returns aggregate stored bytes.
func (ms *MessageStore) TotalBytes() int64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var n int64
	for _, ent := range ms.entries {
		n += ent.Size
	}
	return n
}

// Weight returns the cull weight for one entry (lower is kept longer).
// The age weight divisor matches upstream at four days.
func (ms *MessageStore) Weight(transientID []byte) float64 {
	ent, ok := ms.Get(transientID)
	if !ok {
		return 0
	}
	now := time.Now().Unix()
	// Upstream uses float division so age weight is fractional.
	ageWeight := math.Max(1, float64(now-int64(ent.ReceivedAt))/(4*24*60*60))
	priority := 1.0
	if key, ok := destIDFrom(ent.DestinationHash); ok {
		if _, hit := ms.prioritisedDest[key]; hit {
			priority = 0.1
		}
	}
	return priority * ageWeight * float64(ent.Size)
}

// StampValue returns the stored stamp value for transientID.
func (ms *MessageStore) StampValue(transientID []byte) (int64, bool) {
	ent, ok := ms.Get(transientID)
	if !ok {
		return 0, false
	}
	return ent.StampValue, true
}

// CleanExpired removes entries older than messageExpirySeconds.
func (ms *MessageStore) CleanExpired() int {
	now := time.Now().Unix()
	removed := 0
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for key, ent := range ms.entries {
		if now <= int64(ent.ReceivedAt)+messageExpirySeconds {
			continue
		}
		delete(ms.entries, key)
		if ent.FilePath != "" {
			_ = os.Remove(ent.FilePath)
		}
		removed++
	}
	return removed
}

// CleanToLimit removes lowest-priority entries until within limitBytes.
func (ms *MessageStore) CleanToLimit() int64 {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.enforceLimitLocked()
}

func (ms *MessageStore) enforceLimitLocked() int64 {
	if ms.limitBytes <= 0 {
		return 0
	}
	total := int64(0)
	for _, ent := range ms.entries {
		total += ent.Size
	}
	if total <= ms.limitBytes {
		return 0
	}
	needed := total - ms.limitBytes
	type weighted struct {
		key    string
		weight float64
		size   int64
	}
	ws := make([]weighted, 0, len(ms.entries))
	for key, ent := range ms.entries {
		now := time.Now().Unix()
		ageWeight := math.Max(1, float64(now-int64(ent.ReceivedAt))/(4*24*60*60))
		priority := 1.0
		if key, ok := destIDFrom(ent.DestinationHash); ok {
			if _, hit := ms.prioritisedDest[key]; hit {
				priority = 0.1
			}
		}
		ws = append(ws, weighted{
			key:    key,
			weight: priority * ageWeight * float64(ent.Size),
			size:   ent.Size,
		})
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].weight > ws[j].weight })

	cleaned := int64(0)
	for _, w := range ws {
		if cleaned >= needed {
			break
		}
		ent := ms.entries[w.key]
		delete(ms.entries, w.key)
		if ent != nil && ent.FilePath != "" {
			_ = os.Remove(ent.FilePath)
		}
		cleaned += w.size
	}
	return cleaned
}

func (ms *MessageStore) addUnhandledPeer(transientID, peerHash []byte) {
	key := hex.EncodeToString(transientID)
	peerKey := hex.EncodeToString(peerHash)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ent, ok := ms.entries[key]
	if !ok {
		return
	}
	// Upstream only queues a message as unhandled when the peer has not
	// already handled it.
	if slices.Contains(ent.HandledPeers, peerKey) || slices.Contains(ent.UnhandledPeers, peerKey) {
		return
	}
	ent.UnhandledPeers = append(ent.UnhandledPeers, peerKey)
}

// removeUnhandledPeer drops peerHash from the unhandled list only, used
// when a peer must not be offered the entry again without marking it
// handled, matching upstream remove_unhandled_message.
func (ms *MessageStore) removeUnhandledPeer(transientID, peerHash []byte) {
	key := hex.EncodeToString(transientID)
	peerKey := hex.EncodeToString(peerHash)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ent, ok := ms.entries[key]
	if !ok {
		return
	}
	for i, p := range ent.UnhandledPeers {
		if p == peerKey {
			ent.UnhandledPeers = append(ent.UnhandledPeers[:i], ent.UnhandledPeers[i+1:]...)
			break
		}
	}
}

func (ms *MessageStore) markHandled(transientID, peerHash []byte) {
	key := hex.EncodeToString(transientID)
	peerKey := hex.EncodeToString(peerHash)
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ent, ok := ms.entries[key]
	if !ok {
		return
	}
	for i, p := range ent.UnhandledPeers {
		if p == peerKey {
			ent.UnhandledPeers = append(ent.UnhandledPeers[:i], ent.UnhandledPeers[i+1:]...)
			break
		}
	}
	if slices.Contains(ent.HandledPeers, peerKey) {
		return
	}
	ent.HandledPeers = append(ent.HandledPeers, peerKey)
}

func (ms *MessageStore) entriesSnapshot() map[string]*PropagationEntry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	out := make(map[string]*PropagationEntry, len(ms.entries))
	for k, v := range ms.entries {
		out[k] = clonePropagationEntry(v)
	}
	return out
}

func (ms *MessageStore) entryPath(key string) string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ent, ok := ms.entries[key]; ok {
		return ent.FilePath
	}
	return ""
}

func clonePropagationEntry(ent *PropagationEntry) *PropagationEntry {
	if ent == nil {
		return nil
	}
	cp := *ent
	cp.DestinationHash = append([]byte(nil), ent.DestinationHash...)
	cp.HandledPeers = append([]string(nil), ent.HandledPeers...)
	cp.UnhandledPeers = append([]string(nil), ent.UnhandledPeers...)
	return &cp
}
