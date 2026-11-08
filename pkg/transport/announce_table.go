// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"fmt"
	"time"

	"sync/atomic"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"

	"path/filepath"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"

	"github.com/Quad4-Software/Reticulum-Go/internal/sqlitestore"
)

const PathRequestGrace = 400 * time.Millisecond

const announceTableCheckInterval = 250 * time.Millisecond

// announceForwardCheckInterval is how often delayed announce rebroadcasts are
// drained. Must stay well below sim/line hop budgets (pathfinder delay can be 0).
const announceForwardCheckInterval = 25 * time.Millisecond

// cachedAnnounce holds a PATH_RESPONSE-capable announce payload with insert time
// for oldest-first eviction under the in-memory soft cap.
type cachedAnnounce struct {
	pkt *packet.Packet
	at  time.Time
}

func (t *Transport) announcePacketCacheMax() int {
	limit := 0
	if t != nil && t.config != nil {
		limit = t.config.EffectiveMaxInMemoryPaths()
	}
	if limit <= 0 {
		return common.DefaultMaxInMemoryPaths
	}
	return limit
}

// evictAnnouncePacketCacheUnlocked drops oldest cache entries until at or under
// the soft cap. Caller must hold t.mutex.
func (t *Transport) evictAnnouncePacketCacheUnlocked() {
	if t == nil {
		return
	}
	limit := t.announcePacketCacheMax()
	over := len(t.announcePacketCache) - limit
	if over <= 0 {
		return
	}
	batch := max(over, 64)
	if batch > len(t.announcePacketCache) {
		batch = over
	}
	if batch < 1 {
		return
	}
	keys := make([]hash16, 0, batch)
	times := make([]time.Time, 0, batch)
	for k, e := range t.announcePacketCache {
		when := time.Time{}
		if e != nil {
			when = e.at
		}
		if len(keys) < batch {
			keys = append(keys, k)
			times = append(times, when)
			continue
		}
		idx := 0
		newest := times[0]
		for i := 1; i < batch; i++ {
			if times[i].After(newest) {
				newest = times[i]
				idx = i
			}
		}
		if when.Before(newest) {
			keys[idx] = k
			times[idx] = when
		}
	}
	for _, k := range keys {
		delete(t.announcePacketCache, k)
	}
}

// cleanupAnnouncePacketCache removes entries with no live path and enforces the
// soft cap. Called from the maintenance loop after cleanupExpiredPaths.
func (t *Transport) cleanupAnnouncePacketCache() {
	if t == nil {
		return
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	for k := range t.announcePacketCache {
		pathKey := pathMapKey(k.bytes[:])
		p, ok := t.paths[pathKey]
		if !ok || p == nil || pathExpired(p, now) {
			delete(t.announcePacketCache, k)
		}
	}
	t.evictAnnouncePacketCacheUnlocked()
}

func (t *Transport) cacheAnnouncePacket(destHash []byte, pkt *packet.Packet) {
	if t == nil || len(destHash) != packet.TruncatedHashLength || pkt == nil || len(pkt.Data) == 0 {
		return
	}
	key := destKey(destHash)
	t.mutex.Lock()
	if existing := t.announcePacketCache[key]; existing != nil && existing.pkt != nil &&
		bytes.Equal(existing.pkt.Data, pkt.Data) &&
		existing.pkt.Hops == pkt.Hops &&
		existing.pkt.ContextFlag == pkt.ContextFlag {
		existing.at = time.Now()
		t.mutex.Unlock()
		return
	}
	cached := &packet.Packet{
		HeaderType:      pkt.HeaderType,
		PacketType:      pkt.PacketType,
		TransportType:   pkt.TransportType,
		Context:         pkt.Context,
		ContextFlag:     pkt.ContextFlag,
		Hops:            pkt.Hops,
		DestinationType: pkt.DestinationType,
		DestinationHash: append([]byte(nil), destHash...),
		Data:            append([]byte(nil), pkt.Data...),
	}
	t.announcePacketCache[key] = &cachedAnnounce{pkt: cached, at: time.Now()}
	t.evictAnnouncePacketCacheUnlocked()
	db := t.announceCacheDB
	t.mutex.Unlock()
	if db != nil {
		if raw, err := serializeCachedAnnounce(cached); err == nil {
			db.Store(append([]byte(nil), destHash...), raw, "")
			if db.PendingCount() > announceCacheMaxPending {
				_, _ = db.Flush()
			}
		}
	}
}

func (t *Transport) getCachedAnnouncePacket(destHash []byte) *packet.Packet {
	if t == nil || len(destHash) != packet.TruncatedHashLength {
		return nil
	}
	key := destKey(destHash)
	t.mutex.RLock()
	ca := t.announcePacketCache[key]
	db := t.announceCacheDB
	t.mutex.RUnlock()
	if ca == nil && db != nil {
		if raw, _, ok := db.Fetch(append([]byte(nil), destHash...)); ok {
			if pkt, err := deserializeCachedAnnounce(raw); err == nil {
				ca = &cachedAnnounce{pkt: pkt, at: time.Now()}
				t.mutex.Lock()
				t.announcePacketCache[key] = ca
				t.evictAnnouncePacketCacheUnlocked()
				t.mutex.Unlock()
			}
		}
	}
	if ca == nil {
		return nil
	}
	return ca.pkt
}

// queuePathResponseAnnounce inserts a path-response announce for destHash
// destined to attachedIface. Returns true when an entry was queued.
func (t *Transport) queuePathResponseAnnounce(destHash []byte, path *common.Path, attachedIface common.NetworkInterface, isFromLocalClient bool) bool {
	if t == nil || path == nil || attachedIface == nil || len(destHash) != packet.TruncatedHashLength {
		return false
	}

	key := destKey(destHash)
	t.mutex.RLock()
	var cached *packet.Packet
	if ca := t.announcePacketCache[key]; ca != nil {
		cached = ca.pkt
	}
	announceEntry := t.announceTable[key]
	t.mutex.RUnlock()

	var srcPkt *packet.Packet
	if cached != nil && len(cached.Data) > 0 {
		srcPkt = cached
	} else if announceEntry != nil && announceEntry.Packet != nil {
		srcPkt = announceEntry.Packet
		if len(srcPkt.Data) == 0 && len(srcPkt.Raw) > 0 {
			// Allow test seeds that only set Raw.
			cp := *srcPkt
			if cp.Unpack() != nil {
				cp.Data = append([]byte(nil), srcPkt.Raw...)
				cp.DestinationHash = append([]byte(nil), destHash...)
			}
			srcPkt = &cp
		}
	}
	if srcPkt == nil || (len(srcPkt.Data) == 0 && len(srcPkt.Raw) == 0) {
		return false
	}
	if len(srcPkt.DestinationHash) == 0 {
		cp := *srcPkt
		cp.DestinationHash = append([]byte(nil), destHash...)
		srcPkt = &cp
	}
	if len(srcPkt.Data) == 0 {
		return false
	}

	now := time.Now()
	retransmitTimeout := now.Add(PathRequestGrace)
	if isFromLocalClient {
		retransmitTimeout = now
	}

	entry := &PathAnnounceEntry{
		CreatedAt:         now,
		RetransmitTimeout: retransmitTimeout,
		Retries:           PathfinderR,
		ReceivedFrom:      path.Interface,
		AnnounceHops:      path.HopCount,
		Packet:            srcPkt,
		LocalRebroadcasts: 0,
		BlockRebroadcasts: true,
		AttachedInterface: attachedIface,
	}

	t.mutex.Lock()
	if prev, ok := t.announceTable[key]; ok && prev != nil && prev.AttachedInterface != nil {
		// A later path-request retry must not push RetransmitTimeout out, or
		// aggressive clients never receive a PATH_RESPONSE.
		if prev.BlockRebroadcasts && prev.AttachedInterface == attachedIface {
			t.mutex.Unlock()
			ifaceName := ""
			if attachedIface != nil {
				ifaceName = attachedIface.GetName()
			}
			health.Inc(ifaceName, health.KindPathRespQueuedSkip)
			return true
		}
		t.heldAnnounces[key] = prev
	}
	t.announceTable[key] = entry
	t.mutex.Unlock()

	if isFromLocalClient {
		t.emitAnnounceTableEntry(key, entry)
	}
	return true
}

// notePendingLocalPathRequest records that a shared-instance client is waiting
// for an announce for destHash so a later ingress announce can answer it
// immediately
// maxPendingLocalPathReqs bounds local-client path-request state. Entries
// normally clear on announce answer. A hostile local client could
// otherwise grow the map without limit.
const maxPendingLocalPathReqs = 2048

func (t *Transport) notePendingLocalPathRequest(destHash []byte, iface common.NetworkInterface) {
	if t == nil || iface == nil || len(destHash) != packet.TruncatedHashLength {
		return
	}
	t.mutex.Lock()
	if len(t.pendingLocalPathReqs) >= maxPendingLocalPathReqs {
		t.mutex.Unlock()
		debug.Log(debug.DebugVerbose, "Pending local path request table full, dropping")
		return
	}
	t.pendingLocalPathReqs[destKey(destHash)] = iface
	t.mutex.Unlock()
}

// answerPendingLocalPathRequest sends a PATH_RESPONSE to a waiting local client.
func (t *Transport) answerPendingLocalPathRequest(destHash []byte, hops byte) {
	if t == nil || len(destHash) != packet.TruncatedHashLength {
		return
	}
	key := destKey(destHash)
	t.mutex.Lock()
	iface, ok := t.pendingLocalPathReqs[key]
	if ok {
		delete(t.pendingLocalPathReqs, key)
	}
	var cached *packet.Packet
	if ca := t.announcePacketCache[key]; ca != nil {
		cached = ca.pkt
	}
	path := t.paths[pathMapKey(destHash)]
	t.mutex.Unlock()
	if !ok || iface == nil || cached == nil {
		return
	}
	if path == nil {
		path = &common.Path{HopCount: hops, Interface: iface}
	} else if hops > 0 {
		// Prefer the hops from the announce that just arrived.
		cp := *path
		cp.HopCount = hops
		path = &cp
	}
	_ = t.queuePathResponseAnnounce(destHash, path, iface, true)
}

// processAnnounceTable retransmits queued announce-table entries whose
// retransmit timeout has elapsed.
func (t *Transport) processAnnounceTable() {
	if t == nil {
		return
	}
	now := time.Now()

	t.mutex.Lock()
	type dueItem struct {
		key   hash16
		entry *PathAnnounceEntry
	}
	var due []dueItem
	var completed []hash16

	for key, entry := range t.announceTable {
		if entry == nil {
			completed = append(completed, key)
			continue
		}
		if entry.Retries > 0 && entry.Retries >= LocalRebroadcastsMax {
			completed = append(completed, key)
			continue
		}
		if entry.Retries > PathfinderR {
			completed = append(completed, key)
			continue
		}
		if now.Before(entry.RetransmitTimeout) {
			continue
		}
		entry.RetransmitTimeout = now.Add(time.Duration((PathfinderG+effectivePathfinderRW())*1000.0) * time.Millisecond)
		entry.Retries++
		due = append(due, dueItem{key: key, entry: entry})
	}
	for _, key := range completed {
		delete(t.announceTable, key)
	}
	t.mutex.Unlock()

	for _, item := range due {
		t.emitAnnounceTableEntry(item.key, item.entry)
	}
}

func (t *Transport) emitAnnounceTableEntry(key hash16, entry *PathAnnounceEntry) {
	if entry == nil || entry.Packet == nil || entry.AttachedInterface == nil {
		return
	}
	if !entry.AttachedInterface.IsEnabled() {
		t.mutex.Lock()
		delete(t.announceTable, key)
		t.mutex.Unlock()
		return
	}

	raw, err := t.buildPathResponseWire(entry)
	if err != nil {
		debug.Log(debug.DebugVerbose, "Failed to build path response announce",
			"dest_hash", fmt.Sprintf("%x", entry.Packet.DestinationHash), "error", err)
		return
	}

	if debug.Enabled(debug.DebugInfo) {
		debug.Log(debug.DebugInfo, "Rebroadcasting announce as path response",
			"dest_hash", fmt.Sprintf("%x", entry.Packet.DestinationHash),
			"hops", entry.AnnounceHops,
			"iface", entry.AttachedInterface.GetName())
	}

	if err := sendOnInterface(entry.AttachedInterface, raw, ""); err != nil {
		debug.Log(debug.DebugVerbose, "Failed to send path response announce",
			"iface", entry.AttachedInterface.GetName(), "error", err)
		return
	}
	if sa, ok := entry.AttachedInterface.(interface{ SentAnnounce() }); ok {
		sa.SentAnnounce()
	}

	t.mutex.Lock()
	if held, ok := t.heldAnnounces[key]; ok {
		delete(t.heldAnnounces, key)
		t.announceTable[key] = held
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Reinserting held announce into table",
				"dest_hash", fmt.Sprintf("%x", entry.Packet.DestinationHash))
		}
	} else if cur, ok := t.announceTable[key]; ok && cur == entry && entry.BlockRebroadcasts {
		// Path responses are one-shot for the attached interface.
		delete(t.announceTable, key)
	}
	t.mutex.Unlock()
}

func (t *Transport) buildPathResponseWire(entry *PathAnnounceEntry) ([]byte, error) {
	if t.transportIdentity == nil {
		return nil, fmt.Errorf("no transport identity")
	}
	if entry.Packet == nil || len(entry.Packet.Data) == 0 {
		return nil, fmt.Errorf("empty announce payload")
	}
	destHash := entry.Packet.DestinationHash
	if len(destHash) != packet.TruncatedHashLength {
		return nil, fmt.Errorf("invalid destination hash length")
	}

	ctx := byte(packet.ContextNone)
	if entry.BlockRebroadcasts {
		ctx = packet.ContextPathResponse
	}

	out := &packet.Packet{
		HeaderType:      packet.HeaderType2,
		PacketType:      packet.PacketTypeAnnounce,
		TransportType:   packet.PropagationTransport,
		DestinationType: packet.DestinationSingle,
		DestinationHash: append([]byte(nil), destHash...),
		TransportID:     append([]byte(nil), t.transportIdentity.Hash()...),
		Context:         ctx,
		ContextFlag:     entry.Packet.ContextFlag,
		Data:            append([]byte(nil), entry.Packet.Data...),
		Hops:            entry.AnnounceHops,
	}
	if err := out.Pack(); err != nil {
		return nil, err
	}
	return out.Raw, nil
}

const announceCacheMaxPending = 10000

// serializeCachedAnnounce encodes the announce fields needed to rebuild a
// packet after an announce-cache DB fetch. Node-local format (msgpack map).
func serializeCachedAnnounce(p *packet.Packet) ([]byte, error) {
	return msgpack.Marshal(map[string]any{
		"h": p.HeaderType, "t": p.PacketType, "tt": p.TransportType,
		"c": p.Context, "cf": p.ContextFlag, "hops": p.Hops,
		"dt": p.DestinationType, "dh": p.DestinationHash, "data": p.Data,
	})
}

func deserializeCachedAnnounce(raw []byte) (*packet.Packet, error) {
	var m map[string]any
	if err := msgpack.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	p := &packet.Packet{}
	if v, ok := m["h"].(uint8); ok {
		p.HeaderType = v
	}
	if v, ok := m["t"].(uint8); ok {
		p.PacketType = v
	}
	if v, ok := m["tt"].(uint8); ok {
		p.TransportType = v
	}
	if v, ok := m["c"].(uint8); ok {
		p.Context = v
	}
	if v, ok := m["cf"].(uint8); ok {
		p.ContextFlag = v
	}
	if v, ok := m["hops"].(uint8); ok {
		p.Hops = v
	}
	if v, ok := m["dt"].(uint8); ok {
		p.DestinationType = v
	}
	if v, ok := m["dh"].([]byte); ok {
		p.DestinationHash = append([]byte(nil), v...)
	}
	if v, ok := m["data"].([]byte); ok {
		p.Data = append([]byte(nil), v...)
	}
	return p, nil
}

// openAnnounceCacheDB opens the persistent announce cache at
// <storage>/cache/announces.db (Python 1.6.0 filename) and imports any
// legacy per-hash files under <storage>/cache/announces/. Returns nil on
// any failure so callers fall back to the in-memory cache.
func openAnnounceCacheDB(storagePath string) *sqlitestore.AnnounceCache {
	cacheDir := filepath.Join(storagePath, "cache")
	db, err := sqlitestore.OpenAnnounceCache(filepath.Join(cacheDir, "announces.db"))
	if err != nil {
		debug.Log(debug.DebugWarning, "announce cache DB unavailable, using memory only", "error", err)
		return nil
	}
	if n, err := db.ImportLegacy(filepath.Join(cacheDir, "announces")); err == nil && n > 0 {
		debug.Log(debug.DebugInfo, "imported legacy announce cache files", "count", n)
	}
	return db
}

var announceCacheFlushCounter atomic.Int64

// maintainAnnounceCacheDB flushes the announce-cache DB roughly every 15s
// (Python ac_flush_interval) and cleans entries not backing active paths.
func (t *Transport) maintainAnnounceCacheDB() {
	if t == nil || t.announceCacheDB == nil {
		return
	}
	if announceCacheFlushCounter.Add(1)%3 != 0 { // 5s tick -> ~15s
		return
	}
	t.mutex.RLock()
	active := make([][]byte, 0, len(t.paths))
	for k := range t.paths {
		active = append(active, k[:])
	}
	db := t.announceCacheDB
	t.mutex.RUnlock()
	_, _ = db.Clean(active)
	_, _ = db.Flush()
}

func (t *Transport) closeAnnounceCacheDB() {
	if t != nil && t.announceCacheDB != nil {
		_, _ = t.announceCacheDB.Flush()
		_ = t.announceCacheDB.Close()
		t.announceCacheDB = nil
	}
}
