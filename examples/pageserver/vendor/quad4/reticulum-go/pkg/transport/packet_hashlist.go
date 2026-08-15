// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"sync"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

type packetHashList struct {
	mu   sync.Mutex
	cur  map[[32]byte]struct{}
	prev map[[32]byte]struct{}
}

func newPacketHashList() *packetHashList {
	return &packetHashList{
		cur:  make(map[[32]byte]struct{}),
		prev: make(map[[32]byte]struct{}),
	}
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
	if _, ok := hl.cur[k]; ok {
		return true
	}
	_, ok := hl.prev[k]
	return ok
}

func (hl *packetHashList) add(h []byte) {
	if hl == nil || len(h) == 0 {
		return
	}
	k := hash32FromSlice(h)
	hl.mu.Lock()
	defer hl.mu.Unlock()
	hl.cur[k] = struct{}{}
	if len(hl.cur) > HashlistMaxSize/2 {
		hl.prev = hl.cur
		hl.cur = make(map[[32]byte]struct{})
	}
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
	debug.Log(debug.DebugVerbose, "Filtered duplicate packet",
		"hash", fmt.Sprintf("%x", preview))
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
