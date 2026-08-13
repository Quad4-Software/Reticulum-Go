// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"fmt"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/packet"
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
	for len(t.announcePacketCache) > limit {
		var oldestKey hash16
		var oldestTime time.Time
		first := true
		for k, e := range t.announcePacketCache {
			when := time.Time{}
			if e != nil {
				when = e.at
			}
			if first || when.Before(oldestTime) {
				first = false
				oldestKey = k
				oldestTime = when
			}
		}
		delete(t.announcePacketCache, oldestKey)
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
	t.mutex.Unlock()
}

func (t *Transport) getCachedAnnouncePacket(destHash []byte) *packet.Packet {
	if t == nil || len(destHash) != packet.TruncatedHashLength {
		return nil
	}
	t.mutex.RLock()
	ca := t.announcePacketCache[destKey(destHash)]
	t.mutex.RUnlock()
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
func (t *Transport) notePendingLocalPathRequest(destHash []byte, iface common.NetworkInterface) {
	if t == nil || iface == nil || len(destHash) != packet.TruncatedHashLength {
		return
	}
	t.mutex.Lock()
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
