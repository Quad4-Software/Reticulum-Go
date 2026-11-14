// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
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
	limit   int
}

func newReverseTable(limit int) *reverseTable {
	return &reverseTable{entries: make(map[hash16]*ReverseEntry), limit: limit}
}

func reverseEntriesLimit(cfg *common.ReticulumConfig) int {
	if cfg == nil {
		return 0
	}
	return cfg.MaxReverseEntries
}

func (rt *reverseTable) maxEntries() int {
	if rt != nil && rt.limit != 0 {
		return rt.limit
	}
	return maxReverseEntries
}

// maxReverseEntries bounds the reverse-proof relay table. Entries live up
// to ReverseTimeout, so a flood of transported packets with unique hashes
// would otherwise grow the map unbounded. Oldest entries evict first.
const maxReverseEntries = 32768

// reverseEvictBatch is how many of the oldest entries are dropped each time
// the table overflows. Batching amortizes the O(n) oldest-entry scan over
// thousands of inserts instead of paying it on every put once full.
const reverseEvictBatch = 4096

func (rt *reverseTable) put(truncatedHash []byte, entry *ReverseEntry) {
	if rt == nil || entry == nil || len(truncatedHash) == 0 {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if max := rt.maxEntries(); max > 0 && len(rt.entries) >= max {
		rt.evictOldestLocked()
	}
	rt.entries[hash16FromSlice(truncatedHash)] = entry
}

// evictOldestLocked removes the oldest reverseEvictBatch entries. Caller must
// hold rt.mu. The table only reaches this path when full, so the scan and
// sort run once per batch of inserts rather than once per insert.
func (rt *reverseTable) evictOldestLocked() {
	evictOldestMapKeys(rt.entries,
		func(e *ReverseEntry) time.Time {
			if e == nil {
				return time.Time{}
			}
			return e.Timestamp
		},
		func(e *ReverseEntry) bool { return e != nil },
		reverseEvictBatch)
}

// evictOldestMapKeys deletes the n oldest entries of m according to the
// timestamp returned by at. eligible, when non-nil, restricts candidates
// (for example to unvalidated entries). Sorting once per call amortizes the
// O(n log n) scan over a whole eviction batch instead of paying an O(n)
// scan on every insert while the map sits at its cap.
func evictOldestMapKeys[K comparable, V any](m map[K]V, at func(V) time.Time, eligible func(V) bool, n int) int {
	if n <= 0 || len(m) == 0 {
		return 0
	}
	type candidate struct {
		k K
		t time.Time
	}
	all := make([]candidate, 0, len(m))
	for k, v := range m {
		if eligible != nil && !eligible(v) {
			continue
		}
		all = append(all, candidate{k: k, t: at(v)})
	}
	if len(all) == 0 {
		return 0
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t.Before(all[j].t) })
	if n > len(all) {
		n = len(all)
	}
	for i := 0; i < n; i++ {
		delete(m, all[i].k)
	}
	return n
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

// get reads an entry without consuming it. forwardReverseProof validates the
// receiving interface before popping so a wrong-interface or forged proof
// cannot consume the single-use entry meant for the real proof.
func (rt *reverseTable) get(truncatedHash []byte) (*ReverseEntry, bool) {
	if rt == nil || len(truncatedHash) == 0 {
		return nil, false
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	e, ok := rt.entries[hash16FromSlice(truncatedHash)]
	return e, ok
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
	entry, ok := t.reverseTable.get(dest)
	if !ok || entry == nil {
		return false
	}
	proofForLocalClient := isLocalClientInterface(entry.ReceivedIface)
	if !t.transportEnabled() && !isLocalClientInterface(iface) && !proofForLocalClient {
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
	// Everything matched. Consume the entry now that the proof is honored.
	if popped, ok := t.reverseTable.pop(dest); !ok || popped != entry {
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
