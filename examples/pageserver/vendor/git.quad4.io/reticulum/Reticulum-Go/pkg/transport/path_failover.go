// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"sort"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
)

// Deferred alternate-path state.
//
// Python evaluates every verified announce copy against the path table and
// keeps only the winner, so alternate paths heard via other interfaces are
// discarded. Go dedups announce copies by content hash before verification
// (a CPU win over Python, which verifies every copy), which means the same
// emission arriving on a second interface never reaches the contest.
//
// This file keeps two bounded forms of that discarded information:
//
//   - backupPaths: verified path entries demoted when a better announce
//     replaces them. Promotion needs no network traffic at all.
//   - pendingAltAnnounces: raw copies of the same emission heard via other
//     interfaces. On failover they are re-injected through the normal
//     announce pipeline, which re-runs the contest and verifies on demand.
//
// Both are local state only. Nothing here changes wire behavior.

const (
	// maxBackupPathsPerDest bounds verified alternate paths kept per
	// destination.
	maxBackupPathsPerDest = 3
	// maxPendingAltPerDest bounds buffered same-emission copies per
	// destination (one slot per receiving interface).
	maxPendingAltPerDest = 3
	// maxPendingAltDests bounds the number of destinations holding pending
	// alternate announces.
	maxPendingAltDests = 2048
	// pendingAltMaxBytes caps the raw announce copy size worth buffering.
	pendingAltMaxBytes = 4096
	// pendingAltTTL expires buffered copies so stale alternates cannot be
	// resurrected long after the emission stopped propagating.
	pendingAltTTL = SeenAnnounceTTL
)

// backupPath is a verified path entry kept as a failover candidate after a
// better announce displaced it.
type backupPath struct {
	ifaceName string
	nextHop   []byte
	hops      uint8
	blobs     [][]byte
	at        time.Time
}

// pendingAltAnnounce is a raw announce copy of an already-seen emission
// received on an interface other than the current path's interface.
type pendingAltAnnounce struct {
	data  []byte
	iface string
	hops  byte
	at    time.Time
}

// duplicateAnnounceContest reports whether a deduplicated announce copy
// would win the path-table contest against the current entry. It reads only
// cleartext fields (hops byte, receiving interface, the random blob prefix
// of the signed payload). Winners still pay full verification before any
// path state changes. Returns the current path's interface name so callers
// can tell same-neighbor replays from genuine alternates.
func (t *Transport) duplicateAnnounceContest(hopCount byte, iface common.NetworkInterface, destinationHash, payload []byte) (bool, string) {
	if iface == nil || len(destinationHash) != packet.TruncatedHashLength || len(payload) < 84 {
		return false, ""
	}
	// Announce payload layout: pubKey(64) nameHash(10) randomBlob(10).
	randomHash := payload[74:84]

	announceHops := int(hopCount) + 1
	if isLocalClientInterface(iface) {
		announceHops = int(hopCount)
	}
	if announceHops > MaxHops {
		return false, ""
	}

	key := pathMapKey(destinationHash)
	t.mutex.RLock()
	existing := t.paths[key]
	var curIface common.NetworkInterface
	if existing != nil {
		curIface = existing.Interface
	}
	state := t.pathStates[key]
	t.mutex.RUnlock()

	in := announcePathInput{
		destinationKnown: existing != nil,
		announceHops:     uint8(announceHops),
		randomBlob:       randomHash,
		now:              time.Now(),
		announceAffinity: t.pathingAffinity(iface),
	}
	in.affinityKnown = existing == nil || curIface != nil
	if curIface != nil {
		in.currentAffinity = t.pathingAffinity(curIface)
	}

	curName := ""
	if curIface != nil {
		curName = curIface.GetName()
	}
	return shouldUpdateAnnouncePath(existing, in, state == StateUnresponsive), curName
}

// notePendingAltAnnounce buffers a deduplicated announce copy heard via an
// interface other than the current path's, for later failover. Same-neighbor
// replays are ignored: they cannot offer an alternate path.
func (t *Transport) notePendingAltAnnounce(destinationHash, data []byte, iface common.NetworkInterface, hopCount byte, curIfaceName string) {
	if t == nil || iface == nil || len(destinationHash) != packet.TruncatedHashLength ||
		len(data) == 0 || len(data) > pendingAltMaxBytes {
		return
	}
	ifaceName := iface.GetName()
	if ifaceName == "" || ifaceName == curIfaceName {
		return
	}
	key := pathMapKey(destinationHash)
	now := time.Now()

	t.mutex.Lock()
	defer t.mutex.Unlock()
	if _, ok := t.pendingAltAnnounces[key]; !ok && len(t.pendingAltAnnounces) >= maxPendingAltDests {
		return
	}
	alts := t.pendingAltAnnounces[key]
	for i := range alts {
		if alts[i].iface == ifaceName {
			alts[i] = pendingAltAnnounce{
				data:  append([]byte(nil), data...),
				iface: ifaceName,
				hops:  hopCount,
				at:    now,
			}
			t.pendingAltAnnounces[key] = alts
			return
		}
	}
	if len(alts) >= maxPendingAltPerDest {
		oldest := 0
		for i := range alts {
			if alts[i].at.Before(alts[oldest].at) {
				oldest = i
			}
		}
		alts = append(alts[:oldest], alts[oldest+1:]...)
	}
	t.pendingAltAnnounces[key] = append(alts, pendingAltAnnounce{
		data:  append([]byte(nil), data...),
		iface: ifaceName,
		hops:  hopCount,
		at:    now,
	})
}

// demotePathToBackupUnlocked moves the displaced path entry into the backup
// list. Caller must hold t.mutex.
func (t *Transport) demotePathToBackupUnlocked(key [PathMapKeySize]byte, p *common.Path) {
	if p == nil || p.Interface == nil {
		return
	}
	ifaceName := p.Interface.GetName()
	if ifaceName == "" {
		return
	}
	b := backupPath{
		ifaceName: ifaceName,
		nextHop:   append([]byte(nil), p.NextHop...),
		hops:      p.HopCount,
		blobs:     append([][]byte(nil), p.RandomBlobs...),
		at:        p.LastUpdated,
	}
	backups := t.backupPaths[key]
	for i := range backups {
		if backups[i].ifaceName == ifaceName {
			backups[i] = b
			t.backupPaths[key] = backups
			return
		}
	}
	if len(backups) >= maxBackupPathsPerDest {
		oldest := 0
		for i := range backups {
			if backups[i].at.Before(backups[oldest].at) {
				oldest = i
			}
		}
		backups = append(backups[:oldest], backups[oldest+1:]...)
	}
	t.backupPaths[key] = append(backups, b)
}

// pathNeedsFailoverLocked reports whether the path for key is missing,
// expired, or marked unresponsive. Caller must hold t.mutex (read or write).
func (t *Transport) pathNeedsFailoverLocked(key [PathMapKeySize]byte, now time.Time) bool {
	p := t.paths[key]
	if p == nil || pathExpired(p, now) {
		return true
	}
	return t.pathStates[key] == StateUnresponsive
}

// maybeFailoverPath attempts to restore connectivity for the destination
// behind key after its path failed or expired. Verified backups promote
// instantly. Otherwise the best buffered same-emission alternate is
// re-injected through the normal announce pipeline (contest + verify on
// demand).
func (t *Transport) maybeFailoverPath(key [PathMapKeySize]byte) {
	if t == nil {
		return
	}
	if hops, ok := t.promoteBackupPath(key); ok {
		// A promoted path can answer a waiting local path request right
		// away when the announce bytes are still cached. Otherwise this
		// is a no-op.
		t.answerPendingLocalPathRequest(key[:packet.TruncatedHashLength], hops)
		return
	}
	t.reinjectPendingAlt(key)
}

// promoteBackupPath swaps in the best verified backup path when the current
// path is bad. Returns the promoted hop count and true when a backup was
// promoted.
func (t *Transport) promoteBackupPath(key [PathMapKeySize]byte) (uint8, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()
	if !t.pathNeedsFailoverLocked(key, now) {
		return 0, false
	}
	backups := t.backupPaths[key]
	if len(backups) == 0 {
		return 0, false
	}

	cur := t.paths[key]
	curIfaceName := ""
	if cur != nil && cur.Interface != nil {
		curIfaceName = cur.Interface.GetName()
	}

	// Pick lowest hops, then most recently heard.
	best := -1
	for i := range backups {
		if backups[i].ifaceName == curIfaceName {
			continue
		}
		if best < 0 ||
			backups[i].hops < backups[best].hops ||
			(backups[i].hops == backups[best].hops && backups[i].at.After(backups[best].at)) {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	b := backups[best]
	backups = append(backups[:best], backups[best+1:]...)
	if len(backups) == 0 {
		delete(t.backupPaths, key)
	} else {
		t.backupPaths[key] = backups
	}

	iface, ok := t.interfaces[b.ifaceName]
	if !ok || iface == nil || !iface.IsEnabled() {
		return 0, false
	}

	t.paths[key] = &common.Path{
		NextHop:     append([]byte(nil), b.nextHop...),
		Interface:   iface,
		Hops:        b.hops,
		HopCount:    b.hops,
		LastUpdated: now,
		RandomBlobs: append([][]byte(nil), b.blobs...),
		Expires:     now.Add(pathLifetimeFor(iface)),
	}
	t.pathStates[key] = StateUnknown
	t.markPathTableDirty()

	if debug.Enabled(debug.DebugInfo) {
		debug.Log(debug.DebugInfo, "Promoted backup path after failure",
			"dest_hash", debug.Hex(key[:8]), "iface", b.ifaceName, "hops", b.hops)
	}
	return b.hops, true
}

// reinjectPendingAlt pops buffered same-emission alternates for key and
// replays the best candidates through handleAnnouncePacket. Candidates that
// lose the contest again re-buffer themselves via notePendingAltAnnounce;
// candidates never reached because an earlier one restored the path are
// re-added directly.
func (t *Transport) reinjectPendingAlt(key [PathMapKeySize]byte) {
	t.mutex.Lock()
	alts := t.pendingAltAnnounces[key]
	if len(alts) == 0 {
		t.mutex.Unlock()
		return
	}
	delete(t.pendingAltAnnounces, key)
	t.mutex.Unlock()

	now := time.Now()
	fresh := alts[:0]
	for _, a := range alts {
		if now.Sub(a.at) <= pendingAltTTL {
			fresh = append(fresh, a)
		}
	}
	if len(fresh) == 0 {
		return
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].hops != fresh[j].hops {
			return fresh[i].hops < fresh[j].hops
		}
		return fresh[i].at.After(fresh[j].at)
	})

	tried := 0
	for _, alt := range fresh {
		if !t.pathNeedsFailover(key) {
			break
		}
		tried++
		t.mutex.RLock()
		iface, ok := t.interfaces[alt.iface]
		t.mutex.RUnlock()
		if !ok || iface == nil || !iface.IsEnabled() {
			continue
		}
		if err := t.handleAnnouncePacket(alt.data, iface); err != nil && debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Pending alternate announce re-injection failed",
				"iface", alt.iface, "error", err)
		}
	}
	if tried < len(fresh) {
		t.rebufferPendingAlt(key, fresh[tried:])
	}
}

// rebufferPendingAlt returns untried candidates to the pending alternate
// table, preserving the per-interface dedupe and per-destination cap.
func (t *Transport) rebufferPendingAlt(key [PathMapKeySize]byte, alts []pendingAltAnnounce) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if _, ok := t.pendingAltAnnounces[key]; !ok && len(t.pendingAltAnnounces) >= maxPendingAltDests {
		return
	}
	cur := t.pendingAltAnnounces[key]
	for _, a := range alts {
		replaced := false
		for i := range cur {
			if cur[i].iface == a.iface {
				cur[i] = a
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		if len(cur) >= maxPendingAltPerDest {
			oldest := 0
			for i := range cur {
				if cur[i].at.Before(cur[oldest].at) {
					oldest = i
				}
			}
			cur = append(cur[:oldest], cur[oldest+1:]...)
		}
		cur = append(cur, a)
	}
	t.pendingAltAnnounces[key] = cur
}

// pathNeedsFailover reports whether the path for key still needs failover.
func (t *Transport) pathNeedsFailover(key [PathMapKeySize]byte) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.pathNeedsFailoverLocked(key, time.Now())
}
