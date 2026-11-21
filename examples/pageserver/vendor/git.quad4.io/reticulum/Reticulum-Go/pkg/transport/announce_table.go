// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"time"

	"sync/atomic"

	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"

	"path/filepath"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/health"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"

	"git.quad4.io/reticulum/Reticulum-Go/internal/sqlitestore"
)

const PathRequestGrace = 400 * time.Millisecond

const announceTableCheckInterval = 250 * time.Millisecond

// announceForwardCheckInterval is how often delayed announce rebroadcasts are
// drained. Must stay well below sim/line hop budgets (pathfinder delay can be 0).
const announceForwardCheckInterval = 25 * time.Millisecond

// cachedAnnounce holds the wire fields needed to rebuild an announce
// packet for PATH_RESPONSE answers. Storing the fields instead of a
// whole *packet.Packet keeps the per-destination cost to the payload
// plus this struct rather than the packet's slice-heavy layout.
type cachedAnnounce struct {
	data            []byte
	raw             []byte // wire bytes when the seed carried Raw without Data
	headerType      byte
	packetType      byte
	transportType   byte
	context         byte
	contextFlag     byte
	hops            byte
	destinationType byte
	at              time.Time
}

func newCachedAnnounce(p *packet.Packet) *cachedAnnounce {
	var raw []byte
	if len(p.Data) == 0 && len(p.Raw) > 0 {
		// Test seeds may carry wire bytes without a decoded payload.
		raw = append([]byte(nil), p.Raw...)
	}
	return &cachedAnnounce{
		data:            append([]byte(nil), p.Data...),
		raw:             raw,
		headerType:      p.HeaderType,
		packetType:      p.PacketType,
		transportType:   p.TransportType,
		context:         p.Context,
		contextFlag:     p.ContextFlag,
		hops:            p.Hops,
		destinationType: p.DestinationType,
	}
}

// packet rebuilds a *packet.Packet for emit. The returned packet owns a
// fresh DestinationHash but shares data: callers must not mutate it.
func (ca *cachedAnnounce) packet(destHash []byte) *packet.Packet {
	return &packet.Packet{
		HeaderType:      ca.headerType,
		PacketType:      ca.packetType,
		TransportType:   ca.transportType,
		Context:         ca.context,
		ContextFlag:     ca.contextFlag,
		Hops:            ca.hops,
		DestinationType: ca.destinationType,
		DestinationHash: append([]byte(nil), destHash...),
		Data:            ca.data,
		Raw:             ca.raw,
	}
}

func (t *Transport) announcePacketCacheMax() int {
	if t != nil && t.config != nil && t.config.MaxAnnounceCache != 0 {
		if t.config.MaxAnnounceCache < 0 {
			return math.MaxInt32
		}
		return t.config.MaxAnnounceCache
	}
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
	// Evict down to 90% of the cap in one sorted pass so the O(n log n)
	// scan amortizes over a batch of inserts instead of running on every
	// cacheAnnouncePacket once the cache sits at its soft cap.
	need := len(t.announcePacketCache) - limit*9/10
	if need < 1 {
		need = 1
	}
	evictOldestMapKeys(t.announcePacketCache,
		func(e *cachedAnnounce) time.Time {
			if e == nil {
				return time.Time{}
			}
			return e.at
		},
		nil, need)
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
	t.announcePacketCache = shrinkSparseMap(t.announcePacketCache, &t.announceCachePeak)
}

func (t *Transport) cacheAnnouncePacket(destHash []byte, pkt *packet.Packet) {
	if t == nil || len(destHash) != packet.TruncatedHashLength || pkt == nil || len(pkt.Data) == 0 {
		return
	}
	key := destKey(destHash)
	t.mutex.Lock()
	if existing := t.announcePacketCache[key]; existing != nil &&
		bytes.Equal(existing.data, pkt.Data) &&
		existing.hops == pkt.Hops &&
		existing.contextFlag == pkt.ContextFlag {
		existing.at = time.Now()
		t.mutex.Unlock()
		return
	}
	ca := newCachedAnnounce(pkt)
	ca.at = time.Now()
	t.announcePacketCache[key] = ca
	t.evictAnnouncePacketCacheUnlocked()
	db := t.announceCacheDB
	t.mutex.Unlock()
	if db != nil {
		if raw, err := serializeCachedAnnounce(destHash, ca); err == nil {
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
			if loaded, err := deserializeCachedAnnounce(raw); err == nil {
				ca = loaded
				ca.at = time.Now()
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
	return ca.packet(destHash)
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
		cached = ca.packet(destHash)
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
		cached = ca.packet(destHash)
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
		cp := path.Clone()
		cp.HopCount = hops
		path = cp
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
func serializeCachedAnnounce(destHash []byte, ca *cachedAnnounce) ([]byte, error) {
	return msgpack.Marshal(map[string]any{
		"h": ca.headerType, "t": ca.packetType, "tt": ca.transportType,
		"c": ca.context, "cf": ca.contextFlag, "hops": ca.hops,
		"dt": ca.destinationType, "dh": destHash, "data": ca.data,
	})
}

func deserializeCachedAnnounce(raw []byte) (*cachedAnnounce, error) {
	var m map[string]any
	if err := msgpack.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	ca := &cachedAnnounce{}
	if v, ok := m["h"].(uint8); ok {
		ca.headerType = v
	}
	if v, ok := m["t"].(uint8); ok {
		ca.packetType = v
	}
	if v, ok := m["tt"].(uint8); ok {
		ca.transportType = v
	}
	if v, ok := m["c"].(uint8); ok {
		ca.context = v
	}
	if v, ok := m["cf"].(uint8); ok {
		ca.contextFlag = v
	}
	if v, ok := m["hops"].(uint8); ok {
		ca.hops = v
	}
	if v, ok := m["dt"].(uint8); ok {
		ca.destinationType = v
	}
	if v, ok := m["data"].([]byte); ok {
		ca.data = append([]byte(nil), v...)
	}
	return ca, nil
}

// openAnnounceCacheDB opens the persistent announce cache at
// <storage>/cache/announces.db (Python 1.6.0 filename) and imports any
// legacy per-hash files under <storage>/cache/announces/. Returns nil on
// any failure so callers fall back to the in-memory cache.
func openAnnounceCacheDB(storagePath string) *sqlitestore.AnnounceCache {
	if !sqlitestore.Available {
		return nil
	}
	cacheDir := filepath.Join(storagePath, "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		debug.Log(debug.DebugWarning, "announce cache DB unavailable, using memory only", "error", err)
		return nil
	}
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
