// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
)

const (
	rebalanceWindow     = time.Minute
	rebalanceMaxPerDest = 8
)

type rebalanceEntry struct {
	windowStart time.Time
	count       int
}

// LinkPathRebalanceAllowed reports whether LRPROOF hop rebalancing is enabled.
func (t *Transport) LinkPathRebalanceAllowed() bool {
	if t == nil || t.config == nil {
		return true
	}
	if t.config.AllowLinkPathRebalanceSet {
		return t.config.AllowLinkPathRebalance
	}
	return true
}

// RebalancePathHops updates path-table hop counts after a cryptographically
// validated LRPROOF (RNS 1.4.1). See applyPathHopRebalance for Go uniqueness.
func (t *Transport) RebalancePathHops(destHash []byte, newHops uint8, proofIface common.NetworkInterface) bool {
	return t.applyPathHopRebalance(destHash, newHops, proofIface)
}

// allowRebalanceDest applies Go-unique dampening so thrashing topologies cannot
// rewrite hop counts unbounded times per destination.
func (t *Transport) allowRebalanceDest(destHash []byte) bool {
	if t == nil || len(destHash) == 0 {
		return false
	}
	key := pathMapKey(destHash)
	now := time.Now()
	t.rebalanceMu.Lock()
	defer t.rebalanceMu.Unlock()
	if t.rebalanceByDest == nil {
		t.rebalanceByDest = make(map[[PathMapKeySize]byte]*rebalanceEntry)
	}
	e := t.rebalanceByDest[key]
	if e == nil || now.Sub(e.windowStart) > rebalanceWindow {
		t.rebalanceByDest[key] = &rebalanceEntry{windowStart: now, count: 1}
		return true
	}
	if e.count >= rebalanceMaxPerDest {
		return false
	}
	e.count++
	return true
}

// applyPathHopRebalance updates path-table hop counts after a cryptographically
// validated LRPROOF (RNS 1.4.1). Remaining hops on the link table are updated
// by the caller. When no path entry exists, this still returns true so transit
// RemainingHops can be corrected (Python updates rem hops even without a path).
//
// Go uniqueness: refuse hop increases when the current path interface still
// has substantially higher effective gravity than the proof ingress (keeps
// high-gravity static paths sticky under chaos), and dampen thrashing.
func (t *Transport) applyPathHopRebalance(destHash []byte, newHops uint8, proofIface common.NetworkInterface) bool {
	if t == nil || len(destHash) == 0 {
		return false
	}
	if !t.allowRebalanceDest(destHash) {
		debug.Log(debug.DebugInfo, "Path rebalance dampened",
			"dest", pathHex(destHash))
		return false
	}
	key := pathMapKey(destHash)
	t.mutex.Lock()
	defer t.mutex.Unlock()
	path := t.paths[key]
	ifaceName := ""
	if proofIface != nil {
		ifaceName = proofIface.GetName()
	}
	if path == nil {
		health.Inc(ifaceName, health.KindPathRebalance)
		debug.Log(debug.DebugInfo, "Allowing LRPROOF hop rebalance without path entry",
			"dest", pathHex(destHash),
			"to", newHops)
		return true
	}
	oldHops := path.HopCount
	if newHops > oldHops && path.Interface != nil && proofIface != nil {
		curAff := t.pathingAffinityLocked(path.Interface)
		proofAff := t.pathingAffinityLocked(proofIface)
		if curAff >= proofAff+10 {
			debug.Log(debug.DebugInfo, "Refusing hop-increasing rebalance onto lower-gravity ingress",
				"dest", pathHex(destHash),
				"old_hops", oldHops,
				"new_hops", newHops,
				"path_gravity", curAff,
				"proof_gravity", proofAff)
			return false
		}
	}
	path.HopCount = newHops
	path.Hops = newHops
	path.LastUpdated = time.Now()
	t.markPathTableDirty()
	health.Inc(ifaceName, health.KindPathRebalance)
	debug.Log(debug.DebugInfo, "Re-balancing path from link-request proof",
		"dest", pathHex(destHash),
		"from", oldHops,
		"to", newHops)
	return true
}

func (t *Transport) pathingAffinityLocked(iface common.NetworkInterface) int {
	g := interfaceGravity(iface)
	if t == nil || iface == nil {
		return g
	}
	name := iface.GetName()
	t.rebalanceMu.Lock()
	e := t.ifacePenalties[name]
	pen := 0
	if e != nil {
		pen = e.penalty
	}
	t.rebalanceMu.Unlock()
	if pen <= 0 {
		return g
	}
	if pen > 32 {
		pen = 32
	}
	return g - pen
}

func pathHex(h []byte) string {
	if len(h) > 8 {
		h = h[:8]
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(h)*2)
	for i, b := range h {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}

// ensureRebalanceState initializes rebalance maps. Called from NewTransport.
func (t *Transport) ensureRebalanceState() {
	t.rebalanceByDest = make(map[[PathMapKeySize]byte]*rebalanceEntry)
	t.ifacePenalties = make(map[string]*ifacePenalty)
}
