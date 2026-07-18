// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
)

// ReverseEntry stores the return path for a transported DATA packet so
// receipt proofs can be forwarded back toward the originator.
type ReverseEntry struct {
	ReceivedIface common.NetworkInterface
	OutboundIface common.NetworkInterface
	Timestamp     time.Time
}

type reverseTable struct {
	mu      sync.Mutex
	entries map[hash16]*ReverseEntry
}

func newReverseTable() *reverseTable {
	return &reverseTable{entries: make(map[hash16]*ReverseEntry)}
}

func (rt *reverseTable) put(truncatedHash []byte, entry *ReverseEntry) {
	if rt == nil || entry == nil || len(truncatedHash) == 0 {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.entries[hash16FromSlice(truncatedHash)] = entry
}

func (rt *reverseTable) pop(truncatedHash []byte) (*ReverseEntry, bool) {
	if rt == nil || len(truncatedHash) == 0 {
		return nil, false
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	k := hash16FromSlice(truncatedHash)
	e, ok := rt.entries[k]
	if !ok {
		return nil, false
	}
	delete(rt.entries, k)
	return e, true
}

func (rt *reverseTable) sweep(maxAge time.Duration) int {
	if rt == nil {
		return 0
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	now := time.Now()
	removed := 0
	for k, e := range rt.entries {
		if e == nil || now.Sub(e.Timestamp) > maxAge {
			delete(rt.entries, k)
			removed++
		}
	}
	return removed
}

func (rt *reverseTable) removeEntriesReferencing(iface common.NetworkInterface) {
	if rt == nil || iface == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for k, e := range rt.entries {
		if e == nil {
			continue
		}
		if e.ReceivedIface == iface || e.OutboundIface == iface {
			delete(rt.entries, k)
		}
	}
}

func (t *Transport) recordReverseEntry(pkt *packet.Packet, recvIface, outIface common.NetworkInterface) {
	if t == nil || t.reverseTable == nil || pkt == nil || recvIface == nil || outIface == nil {
		return
	}
	th := pkt.TruncatedHash()
	if len(th) == 0 {
		return
	}
	t.reverseTable.put(th, &ReverseEntry{
		ReceivedIface: recvIface,
		OutboundIface: outIface,
		Timestamp:     time.Now(),
	})
}

// forwardReverseProof returns a receipt proof toward the originator when this
// node previously transported the corresponding DATA packet.
func (t *Transport) forwardReverseProof(pkt *packet.Packet, iface common.NetworkInterface) bool {
	if t == nil || t.reverseTable == nil || pkt == nil {
		return false
	}
	dest := pkt.DestinationHash
	if len(dest) > packet.TruncatedHashLength {
		dest = dest[:packet.TruncatedHashLength]
	}
	entry, ok := t.reverseTable.pop(dest)
	if !ok || entry == nil {
		return false
	}
	proofForLocalClient := isLocalClientInterface(entry.ReceivedIface)
	if !t.transportEnabled() && !isLocalClientInterface(iface) && !proofForLocalClient {
		t.reverseTable.put(dest, entry)
		return false
	}
	if iface != entry.OutboundIface {
		debug.Log(debug.DebugInfo, "Proof received on wrong interface for reverse path",
			"dest_hash", fmt.Sprintf("%x", dest))
		return true
	}
	if entry.ReceivedIface == nil || !entry.ReceivedIface.IsEnabled() {
		return true
	}
	out := rewriteHopsOnly(pkt.Raw, pkt.Hops)
	debug.Log(debug.DebugInfo, "Transporting receipt proof via reverse table",
		"dest_hash", fmt.Sprintf("%x", dest),
		"out_iface", entry.ReceivedIface.GetName())
	if err := sendOnInterface(entry.ReceivedIface, out, ""); err != nil {
		debug.Log(debug.DebugError, "Failed to transport receipt proof", "error", err)
	}
	return true
}
