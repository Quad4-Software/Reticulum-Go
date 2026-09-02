// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

type hashGen struct {
	slots [][32]byte
	occ   []byte
	n     int
	mask  int
}

type packetHashList struct {
	mu   sync.Mutex
	max  int
	cur  hashGen
	prev hashGen
}

func hashSlot(k [32]byte, mask int) int {
	x := binary.LittleEndian.Uint64(k[:8])
	x ^= binary.LittleEndian.Uint64(k[8:16])
	x ^= binary.LittleEndian.Uint64(k[16:24])
	x ^= binary.LittleEndian.Uint64(k[24:32])
	if mask < 0 {
		return 0
	}
	slot := x & uint64(mask)
	if slot > uint64(math.MaxInt) {
		return 0
	}
	return int(slot)
}

func (g *hashGen) has(k [32]byte) bool {
	if g == nil || len(g.slots) == 0 {
		return false
	}
	i := hashSlot(k, g.mask)
	for {
		if g.occ[i] == 0 {
			return false
		}
		if g.slots[i] == k {
			return true
		}
		i = (i + 1) & g.mask
	}
}

func (g *hashGen) grow() {
	ns := 8
	if len(g.slots) > 0 {
		ns = len(g.slots) * 2
	}
	ng := hashGen{
		slots: make([][32]byte, ns),
		occ:   make([]byte, ns),
		mask:  ns - 1,
	}
	for i, occ := range g.occ {
		if occ != 0 {
			ng.insert(g.slots[i])
		}
	}
	*g = ng
}

func (g *hashGen) insert(k [32]byte) {
	i := hashSlot(k, g.mask)
	for {
		if g.occ[i] == 0 {
			g.slots[i] = k
			g.occ[i] = 1
			g.n++
			return
		}
		if g.slots[i] == k {
			return
		}
		i = (i + 1) & g.mask
	}
}

func (g *hashGen) put(k [32]byte) {
	if g == nil {
		return
	}
	if len(g.slots) == 0 || (g.n+1)*2 > len(g.slots) {
		g.grow()
	}
	g.insert(k)
}

func newPacketHashList(limit int) *packetHashList {
	if limit <= 0 {
		limit = HashlistMaxSize
	}
	return &packetHashList{max: limit}
}

func hash32FromSlice(h []byte) [32]byte {
	var out [32]byte
	copy(out[:], h)
	return out
}

func (hl *packetHashList) seen(h []byte) bool {
	if hl == nil || len(h) == 0 {
		return false
	}
	k := hash32FromSlice(h)
	hl.mu.Lock()
	defer hl.mu.Unlock()
	return hl.cur.has(k) || hl.prev.has(k)
}

func (hl *packetHashList) add(h []byte) {
	if hl == nil || len(h) == 0 {
		return
	}
	k := hash32FromSlice(h)
	hl.mu.Lock()
	defer hl.mu.Unlock()
	hl.cur.put(k)
	rotateAt := max(hl.max/2, 1)
	if hl.cur.n > rotateAt {
		hl.prev = hl.cur
		hl.cur = hashGen{}
	}
}

// Len returns the number of hashes in the current and previous generations.
func (hl *packetHashList) Len() int {
	if hl == nil {
		return 0
	}
	hl.mu.Lock()
	defer hl.mu.Unlock()
	return hl.cur.n + hl.prev.n
}

func effectivePacketHashlistMax(cfg *common.ReticulumConfig) int {
	if cfg == nil {
		return common.DefaultMaxPacketHashlistClient
	}
	return cfg.EffectiveMaxPacketHashlist()
}

// packetFilter mirrors Python Transport.packet_filter for duplicate detection
// and context allow-lists. Caller must have unpacked pkt.
func (t *Transport) packetFilter(pkt *packet.Packet) bool {
	if t == nil || pkt == nil {
		return false
	}
	if t.config != nil && t.config.ConnectedToSharedInstance {
		return true
	}

	switch pkt.Context {
	case packet.ContextKeepalive, packet.ContextResourceReq, packet.ContextResourcePRF,
		packet.ContextResource, packet.ContextCacheReq, packet.ContextChannel:
		return true
	}

	if pkt.DestinationType == packet.DestinationPlain || pkt.DestinationType == packet.DestinationGroup {
		if pkt.PacketType == packet.PacketTypeAnnounce {
			return false
		}
		if pkt.Hops > 1 {
			return false
		}
		return true
	}

	h := pkt.GetHash()
	if t.packetHashes == nil || !t.packetHashes.seen(h) {
		return true
	}
	if pkt.PacketType == packet.PacketTypeAnnounce && pkt.DestinationType == packet.DestinationSingle {
		return true
	}
	preview := h
	if len(preview) > 8 {
		preview = preview[:8]
	}
	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Filtered duplicate packet",
			"hash", fmt.Sprintf("%x", preview))
	}
	return false
}

// maybeRememberPacketHash records a packet hash unless deferred for link-table
// transit or LRPROOF (Python inbound remember_packet_hash rules).
func (t *Transport) maybeRememberPacketHash(pkt *packet.Packet) {
	if t == nil || pkt == nil || t.packetHashes == nil {
		return
	}
	if t.config != nil && t.config.ConnectedToSharedInstance {
		return
	}
	dest := pkt.DestinationHash
	if len(dest) > packet.TruncatedHashLength {
		dest = dest[:packet.TruncatedHashLength]
	}
	if t.linkTable != nil {
		if _, ok := t.linkTable.get(dest); ok {
			return
		}
	}
	if pkt.PacketType == packet.PacketTypeProof && pkt.Context == packet.ContextLRProof {
		return
	}
	t.packetHashes.add(pkt.GetHash())
}

// rememberPacketHashForced adds a hash after deferred LRPROOF validation or
// link-table transit completes.
func (t *Transport) rememberPacketHashForced(pkt *packet.Packet) {
	if t == nil || pkt == nil || t.packetHashes == nil {
		return
	}
	if t.config != nil && t.config.ConnectedToSharedInstance {
		return
	}
	t.packetHashes.add(pkt.GetHash())
}
