// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"time"

	"quad4/bzip2/pkg/bzip2"
	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

const (
	hashmapNotExhausted = 0x00
	hashmapExhausted    = 0xff

	// incomingResourceRetryInterval is how often the receive-side watchdog
	// polls for a stalled transfer.
	incomingResourceRetryInterval = 100 * time.Millisecond

	// incomingResourceStallGrace is the minimum idle time after the last
	// accepted part or HMU before a re-request.
	incomingResourceStallGrace = 250 * time.Millisecond

	// incomingResourceWindowShrinkStalls is how many consecutive stalls are
	// required before shrinking the current request window.
	incomingResourceWindowShrinkStalls = 2

	// incomingResourceWindowMaxShrinkStalls is how many consecutive stalls
	// are required before shrinking windowMax (keeps the fast ceiling
	// unless the path is repeatedly unhealthy).
	incomingResourceWindowMaxShrinkStalls = 3
)

// incomingPartPool recycles part payload buffers. ContextResource payloads
// alias packet buffers, so each accepted part must be copied before the
// packet is recycled by the transport.
var incomingPartPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 512)
	},
}

func cloneIncomingPart(data []byte) []byte {
	b, _ := incomingPartPool.Get().([]byte)
	if cap(b) < len(data) {
		b = make([]byte, len(data))
	} else {
		b = b[:len(data)]
	}
	copy(b, data)
	return b
}

func releaseIncomingPart(b []byte) {
	if b == nil {
		return
	}
	incomingPartPool.Put(b[:0])
}

// IncomingResource is delivered to resource concluded callbacks when the
// transfer carried metadata (rncp file name). Plain transfers still deliver
// []byte for backward compatibility with existing callbacks.
type IncomingResource struct {
	Data     []byte
	Metadata map[string]any
	Hash     []byte
}

type mapHashKey [resource.MapHashLen]byte

type incomingResourceAsm struct {
	adv *resource.ResourceAdvertisement
	sdu int

	// hashmapSegLen is the number of hash entries per hashmap/HMU segment.
	// It must be computed from the link MDU, matching both
	// ResourceAdvertisement.Pack (the initial segment 0) and
	// chooseHashmapUpdateSegment/HashmapSegment on the sending side
	// (subsequent HMU segments) -- NOT from the resource part SDU (sdu
	// above), which is a different, smaller value used purely for sizing
	// actual part payloads. Using the wrong value here silently
	// desynchronizes every segment offset beyond the first (whose offset
	// is always zero regardless of segment size), permanently gapping the
	// receiver's hashmap and stalling the transfer forever.
	hashmapSegLen int

	partSlots            [][]byte
	mapHashes            [][]byte
	inflight             []bool
	hashIndex            map[mapHashKey][]int
	hashmapHeight        int
	totalParts           int
	consecutiveCompleted int
	waitingForHmu        bool
	outstandingParts     int
	window               int
	windowMin            int
	windowMax            int
	consecutiveStalls    int
	lastProgressAt       time.Time

	startedAt      time.Time
	reqRounds      int
	hmuRounds      int
	stallRetries   int
	partsReceived  int
	bytesReceived  int64
	hmuWaitNanos   int64
	hmuWaitStarted time.Time
}

func (rx *incomingResourceAsm) applyHashmapSegment(segment int, hashmapBytes []byte) int {
	segLen := rx.hashmapSegLen
	if segLen <= 0 {
		segLen = 1
	}
	added := 0
	hashes := len(hashmapBytes) / resource.MapHashLen
	for i := range hashes {
		idx := i + segment*segLen
		if idx >= rx.totalParts {
			return added
		}
		if rx.mapHashes[idx] == nil {
			rx.hashmapHeight++
			added++
		}
		off := i * resource.MapHashLen
		mh := append([]byte(nil), hashmapBytes[off:off+resource.MapHashLen]...)
		rx.mapHashes[idx] = mh
		rx.indexMapHash(idx, mh)
	}
	return added
}

func (rx *incomingResourceAsm) indexMapHash(idx int, mh []byte) {
	if rx == nil || len(mh) != resource.MapHashLen {
		return
	}
	if rx.hashIndex == nil {
		rx.hashIndex = make(map[mapHashKey][]int)
	}
	var key mapHashKey
	copy(key[:], mh)
	list := rx.hashIndex[key]
	if slices.Contains(list, idx) {
		return
	}
	rx.hashIndex[key] = append(list, idx)
}

func (rx *incomingResourceAsm) lookupMapHash(mh []byte, windowStart, windowEnd int) int {
	if rx == nil || rx.hashIndex == nil || len(mh) != resource.MapHashLen {
		return -1
	}
	var key mapHashKey
	copy(key[:], mh)
	for _, i := range rx.hashIndex[key] {
		if i < windowStart || i >= windowEnd {
			continue
		}
		if rx.partSlots[i] != nil {
			continue
		}
		return i
	}
	return -1
}

func (rx *incomingResourceAsm) markHmuWaitLocked(waiting bool) {
	if rx == nil {
		return
	}
	if waiting {
		if !rx.waitingForHmu {
			rx.hmuWaitStarted = time.Now()
		}
		rx.waitingForHmu = true
		return
	}
	if rx.waitingForHmu && !rx.hmuWaitStarted.IsZero() {
		rx.hmuWaitNanos += time.Since(rx.hmuWaitStarted).Nanoseconds()
		rx.hmuWaitStarted = time.Time{}
	}
	rx.waitingForHmu = false
}

func (rx *incomingResourceAsm) knownHashesAhead() int {
	if rx == nil {
		return 0
	}
	start := max(rx.consecutiveCompleted+1, 0)
	n := 0
	for pn := start; pn < rx.totalParts; pn++ {
		if rx.mapHashes[pn] == nil {
			break
		}
		if rx.partSlots[pn] != nil {
			continue
		}
		n++
	}
	return n
}

func (l *Link) flushIncomingResourceStats(rx *incomingResourceAsm, outcome string) {
	if rx == nil {
		return
	}
	wallMs := time.Since(rx.startedAt).Milliseconds()
	if rx.waitingForHmu && !rx.hmuWaitStarted.IsZero() {
		rx.hmuWaitNanos += time.Since(rx.hmuWaitStarted).Nanoseconds()
		rx.hmuWaitStarted = time.Time{}
	}
	debug.Log(
		debug.DebugInfo,
		"Incoming resource transfer stats",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"outcome",
		outcome,
		"parts",
		rx.totalParts,
		"parts_received",
		rx.partsReceived,
		"bytes",
		rx.bytesReceived,
		"wall_ms",
		wallMs,
		"req_rounds",
		rx.reqRounds,
		"hmu_rounds",
		rx.hmuRounds,
		"stall_retries",
		rx.stallRetries,
		"window_final",
		rx.window,
		"window_max",
		rx.windowMax,
		"hmu_wait_ms",
		rx.hmuWaitNanos/int64(time.Millisecond),
	)
	if outcome == "complete" {
		health.Inc(l.attachedIfaceName(), health.KindResourceComplete)
	}
}

func (l *Link) beginIncomingResource(adv *resource.ResourceAdvertisement) error {
	sdu := l.resourceSDU()
	if sdu <= 0 {
		return errors.New("invalid mdu for incoming resource")
	}
	l.mutex.RLock()
	hashmapSegLen := resource.HashmapEntriesPerSegment(l.mdu)
	l.mutex.RUnlock()
	if adv.Parts <= 0 {
		return errors.New("invalid parts in advertisement")
	}
	maxSegmentBytes := int64(resource.MaxEfficientSize) + 4096
	maxParts := min(max(int(maxSegmentBytes/int64(sdu))+8, 1), int(resource.MaxSegments))
	if adv.Parts > maxParts {
		return errors.New("incoming resource parts exceed MaxSegments")
	}
	if adv.TransferSize < 0 {
		return errors.New("incoming resource has negative transfer_size")
	}
	if adv.TransferSize > maxSegmentBytes {
		return errors.New("incoming resource transfer_size exceeds MaxEfficientSize")
	}
	maxTransfer := int64(adv.Parts) * int64(sdu)
	if adv.TransferSize > maxTransfer {
		return errors.New("incoming resource transfer_size exceeds parts*sdu")
	}
	if len(adv.Hashmap) == 0 || len(adv.Hashmap)%resource.MapHashLen != 0 {
		return errors.New("invalid advertisement hashmap")
	}

	now := time.Now()
	rx := &incomingResourceAsm{
		adv:                  adv,
		sdu:                  sdu,
		hashmapSegLen:        hashmapSegLen,
		partSlots:            make([][]byte, adv.Parts),
		mapHashes:            make([][]byte, adv.Parts),
		inflight:             make([]bool, adv.Parts),
		hashIndex:            make(map[mapHashKey][]int),
		totalParts:           adv.Parts,
		consecutiveCompleted: -1,
		window:               resource.WindowMaxSlow,
		windowMin:            resource.WindowMin,
		windowMax:            resource.WindowMaxFast,
		lastProgressAt:       now,
		startedAt:            now,
	}
	rx.applyHashmapSegment(0, adv.Hashmap)
	debug.Log(
		debug.DebugInfo,
		"Incoming resource started",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"parts",
		adv.Parts,
		"transfer_size",
		adv.TransferSize,
		"hashmap_entries",
		len(adv.Hashmap)/resource.MapHashLen,
	)

	l.incomingMu.Lock()
	l.incomingRx = rx
	l.incomingMu.Unlock()
	go l.watchIncomingResource(rx)
	return l.queueIncomingResourceReqNext()
}

// queueIncomingResourceReqNext sends the next part/HMU request from a fresh
// goroutine so inbound packet handlers can return and keep reading the UDP
// socket instead of nesting ProcessIncoming around an encrypted REQ send.
func (l *Link) queueIncomingResourceReqNext() error {
	go func() {
		if err := l.sendIncomingResourceReqNext(); err != nil {
			debug.Log(
				debug.DebugInfo,
				"Incoming resource request failed",
				"link_id",
				fmt.Sprintf("%x", l.linkID),
				"error",
				err,
			)
		}
	}()
	return nil
}

// watchIncomingResource re-issues the current part/HMU request for rx
// whenever the transfer has made no progress for incomingResourceRetryInterval.
// It exits as soon as rx is no longer the link's active incoming transfer,
// i.e. once it completes, is superseded, or the link is torn down.
func (l *Link) watchIncomingResource(rx *incomingResourceAsm) {
	ticker := time.NewTicker(incomingResourceRetryInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !l.tickIncomingResourceWatchdog(rx) {
			return
		}
	}
}

// tickIncomingResourceWatchdog runs a single watchdog check for rx,
// re-issuing the current part/HMU request if no progress has been made for
// at least incomingResourceRetryInterval. It returns false once the caller
// should stop watching rx: either it is no longer the link's active
// incoming transfer (completed, superseded, or torn down), or a retry
// attempt hard-failed (e.g. the link has gone down).
func (l *Link) tickIncomingResourceWatchdog(rx *incomingResourceAsm) bool {
	l.incomingMu.Lock()
	if l.incomingRx != rx {
		l.incomingMu.Unlock()
		return false
	}
	idle := time.Since(rx.lastProgressAt)
	if idle < incomingResourceStallGrace {
		l.incomingMu.Unlock()
		return true
	}
	// No outstanding work means the next REQ should already be queued from
	// part acceptance. Treat this as an idle gap, not a loss event.
	if rx.outstandingParts == 0 && !rx.waitingForHmu {
		l.incomingMu.Unlock()
		return true
	}
	stalledOnHmu := rx.waitingForHmu
	rx.markHmuWaitLocked(false)
	// Drop in-flight reservations so the re-request can ask for the same
	// missing parts again. Only shrink the window after repeated stalls so
	// a single lost REQ on an otherwise healthy path does not collapse
	// throughput. windowMax shrinks more slowly than the current window.
	for i := range rx.inflight {
		rx.inflight[i] = false
	}
	rx.outstandingParts = 0
	rx.consecutiveStalls++
	rx.stallRetries++
	if rx.consecutiveStalls >= incomingResourceWindowShrinkStalls && rx.window > rx.windowMin && rx.windowMin > 0 {
		rx.window--
	}
	if rx.consecutiveStalls >= incomingResourceWindowMaxShrinkStalls && rx.windowMax > rx.windowMin {
		rx.windowMax--
		if (rx.windowMax - rx.window) > (resource.WindowFlexibility - 1) {
			rx.windowMax--
		}
	}
	l.incomingMu.Unlock()

	debug.Log(
		debug.DebugInfo,
		"Incoming resource stalled, re-requesting",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"idle",
		idle,
		"was_waiting_for_hmu",
		stalledOnHmu,
		"window",
		rx.window,
		"window_max",
		rx.windowMax,
		"consecutive_stalls",
		rx.consecutiveStalls,
	)
	health.Inc(l.attachedIfaceName(), health.KindResourceStall)
	if err := l.sendIncomingResourceReqNext(); err != nil {
		debug.Log(
			debug.DebugInfo,
			"Incoming resource re-request failed",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"error",
			err,
		)
		return false
	}
	return true
}

func (l *Link) sendIncomingResourceReqNext() error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil {
		l.incomingMu.Unlock()
		return nil
	}
	if rx.waitingForHmu {
		l.incomingMu.Unlock()
		return nil
	}

	searchStart := max(rx.consecutiveCompleted+1, 0)
	if searchStart >= rx.totalParts {
		l.incomingMu.Unlock()
		return nil
	}

	win := rx.window
	if win <= 0 {
		win = resource.Window
	}
	// Half-window refill: when outstanding has drained to window/2 or below,
	// request additional hashes up to the configured window total outstanding.
	// Full refill-on-every-part remains forbidden (pipe deadlock).
	if rx.outstandingParts > 0 && rx.outstandingParts > win/2 {
		l.incomingMu.Unlock()
		return nil
	}
	slotsNeeded := win - rx.outstandingParts
	if slotsNeeded <= 0 {
		l.incomingMu.Unlock()
		return nil
	}
	end := min(searchStart+win*2, rx.totalParts)

	requestedHashes := make([]byte, 0, slotsNeeded*resource.MapHashLen)
	requestedIdx := make([]int, 0, slotsNeeded)
	exhausted := false
	for pn := searchStart; pn < end; pn++ {
		if rx.partSlots[pn] != nil || rx.inflight[pn] {
			continue
		}
		mh := rx.mapHashes[pn]
		if mh != nil {
			requestedHashes = append(requestedHashes, mh...)
			requestedIdx = append(requestedIdx, pn)
			if len(requestedIdx) >= slotsNeeded {
				break
			}
			continue
		}
		exhausted = true
		break
	}

	if len(requestedHashes) == 0 && !exhausted {
		l.incomingMu.Unlock()
		return nil
	}

	var prefix []byte
	if exhausted {
		if rx.hashmapHeight == 0 || rx.hashmapHeight-1 >= len(rx.mapHashes) {
			l.incomingMu.Unlock()
			return errors.New("incoming resource cannot request hashmap extension")
		}
		last := rx.mapHashes[rx.hashmapHeight-1]
		if len(last) != resource.MapHashLen {
			l.incomingMu.Unlock()
			return errors.New("invalid last map hash for HMU request")
		}
		prefix = append([]byte{hashmapExhausted}, last...)
		rx.markHmuWaitLocked(true)
		rx.hmuRounds++
		debug.Log(
			debug.DebugInfo,
			"Incoming resource requesting HMU",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"hashmap_height",
			rx.hashmapHeight,
			"anchor_hash",
			fmt.Sprintf("%x", last),
		)
	} else {
		prefix = []byte{hashmapNotExhausted}
	}

	for _, pn := range requestedIdx {
		rx.inflight[pn] = true
	}
	rx.outstandingParts += len(requestedIdx)
	rx.reqRounds++
	reqBody := append(prefix, rx.adv.Hash...)
	reqBody = append(reqBody, requestedHashes...)
	debug.Log(
		debug.DebugVerbose,
		"Incoming resource requesting parts",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"search_start",
		searchStart,
		"window",
		win,
		"requested_parts",
		len(requestedIdx),
		"outstanding_parts",
		rx.outstandingParts,
		"waiting_for_hmu",
		exhausted,
	)
	iface := l.attachedIfaceName()
	l.incomingMu.Unlock()

	if exhausted {
		health.Inc(iface, health.KindResourceHMU)
	}
	if len(requestedIdx) > 0 || exhausted {
		health.Inc(iface, health.KindResourceReq)
	}

	if err := l.SendPacketWithContext(reqBody, packet.ContextResourceReq); err != nil {
		l.incomingMu.Lock()
		if l.incomingRx == rx {
			for _, pn := range requestedIdx {
				if pn >= 0 && pn < len(rx.inflight) {
					rx.inflight[pn] = false
				}
			}
			rx.outstandingParts -= len(requestedIdx)
			if rx.outstandingParts < 0 {
				rx.outstandingParts = 0
			}
			if exhausted {
				rx.markHmuWaitLocked(false)
			}
		}
		l.incomingMu.Unlock()
		return err
	}
	return nil
}

// queueIncomingResourceHMUPrefetch requests the next hashmap segment while
// parts from the current window are still in flight, overlapping HMU RTT
// with data transfer. It does not request additional parts.
func (l *Link) queueIncomingResourceHMUPrefetch() {
	go func() {
		if err := l.sendIncomingResourceHMUPrefetch(); err != nil {
			debug.Log(
				debug.DebugInfo,
				"Incoming resource HMU prefetch failed",
				"link_id",
				fmt.Sprintf("%x", l.linkID),
				"error",
				err,
			)
		}
	}()
}

func (l *Link) sendIncomingResourceHMUPrefetch() error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil || rx.waitingForHmu {
		l.incomingMu.Unlock()
		return nil
	}
	if rx.hashmapHeight >= rx.totalParts {
		l.incomingMu.Unlock()
		return nil
	}
	threshold := max(rx.window/2, 1)
	if rx.knownHashesAhead() >= threshold {
		l.incomingMu.Unlock()
		return nil
	}
	if rx.hashmapHeight == 0 || rx.hashmapHeight-1 >= len(rx.mapHashes) {
		l.incomingMu.Unlock()
		return errors.New("incoming resource cannot prefetch hashmap extension")
	}
	last := rx.mapHashes[rx.hashmapHeight-1]
	if len(last) != resource.MapHashLen {
		l.incomingMu.Unlock()
		return errors.New("invalid last map hash for HMU prefetch")
	}
	rx.markHmuWaitLocked(true)
	rx.hmuRounds++
	reqBody := append([]byte{hashmapExhausted}, last...)
	reqBody = append(reqBody, rx.adv.Hash...)
	iface := l.attachedIfaceName()
	debug.Log(
		debug.DebugInfo,
		"Incoming resource prefetching HMU",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"hashmap_height",
		rx.hashmapHeight,
		"known_ahead",
		rx.knownHashesAhead(),
		"outstanding_parts",
		rx.outstandingParts,
	)
	l.incomingMu.Unlock()

	health.Inc(iface, health.KindResourceHMU)
	health.Inc(iface, health.KindResourceReq)

	if err := l.SendPacketWithContext(reqBody, packet.ContextResourceReq); err != nil {
		l.incomingMu.Lock()
		if l.incomingRx == rx {
			rx.markHmuWaitLocked(false)
		}
		l.incomingMu.Unlock()
		return err
	}
	return nil
}

func (l *Link) resetIncomingResource() {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx != nil {
		l.flushIncomingResourceStats(rx, "reset")
		for i := range rx.partSlots {
			releaseIncomingPart(rx.partSlots[i])
			rx.partSlots[i] = nil
		}
	}
	l.incomingRx = nil
	l.incomingMu.Unlock()
}

func (l *Link) applyIncomingHashmapUpdate(resHash []byte, segment int, hashmapBytes []byte) error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil || rx.adv == nil || !bytes.Equal(resHash, rx.adv.Hash) {
		debug.Log(
			debug.DebugVerbose,
			"Ignoring HMU for inactive/mismatched resource",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
		)
		l.incomingMu.Unlock()
		return nil
	}
	// Match Python Resource.hashmap_update_packet (RNS 1.3.9): ignore HMU
	// unless the receiver is waiting for one.
	if !rx.waitingForHmu {
		l.incomingMu.Unlock()
		return nil
	}
	hashes := len(hashmapBytes) / resource.MapHashLen
	if hashes < 1 {
		cancelHash := append([]byte(nil), rx.adv.Hash...)
		l.incomingMu.Unlock()
		debug.Log(debug.DebugError, "Invalid HMU received, cancelling transfer",
			"link_id", fmt.Sprintf("%x", l.linkID))
		l.resetIncomingResource()
		if l.status.Load() == int32(StatusActive) && len(cancelHash) == sha256.Size {
			_ = l.rejectResource(cancelHash) // #nosec G104 - best effort RESOURCE_RCL
		}
		return errors.New("empty hashmap update")
	}
	added := rx.applyHashmapSegment(segment, hashmapBytes)
	rx.lastProgressAt = time.Now()
	rx.markHmuWaitLocked(false)
	if added == 0 {
		debug.Log(
			debug.DebugVerbose,
			"Incoming duplicate HMU applied",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"segment",
			segment,
			"entries",
			hashes,
			"hashmap_height",
			rx.hashmapHeight,
		)
	} else {
		debug.Log(
			debug.DebugVerbose,
			"Incoming HMU applied",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"segment",
			segment,
			"entries",
			hashes,
			"hashmap_height",
			rx.hashmapHeight,
		)
	}
	// Part REQs stay gated by outstandingParts in sendIncomingResourceReqNext
	// so a late HMU does not open a new part window mid-blast.
	l.incomingMu.Unlock()
	return l.queueIncomingResourceReqNext()
}

func (l *Link) appendIncomingResourcePart(data []byte) error {
	l.incomingMu.Lock()
	rx := l.incomingRx
	if rx == nil {
		l.incomingMu.Unlock()
		return nil
	}

	if len(data) == 0 {
		if l.incomingTransferComplete(rx) {
			inner := l.concatIncomingParts(rx)
			adv := rx.adv
			l.flushIncomingResourceStats(rx, "complete")
			l.incomingRx = nil
			l.incomingMu.Unlock()
			return l.deliverIncomingResource(inner, adv)
		}
		l.incomingMu.Unlock()
		return nil
	}

	rh := rx.adv.RandomHash
	if len(rh) != resource.RandomHashSize {
		l.incomingMu.Unlock()
		return errors.New("bad random hash in advertisement")
	}
	hb := sha256.New()
	hb.Write(data)
	hb.Write(rh)
	sum := hb.Sum(nil)
	mh := sum[:resource.MapHashLen]

	idx := -1
	// Match Python: only consider map hashes inside the active receive
	// window starting at consecutive_completed. A global scan lets 4-byte
	// map-hash collisions steal an earlier empty slot and leave the
	// transfer permanently short one outstanding part.
	windowStart := max(rx.consecutiveCompleted+1, 0)
	win := rx.window
	if win <= 0 {
		win = resource.Window
	}
	windowEnd := min(windowStart+win, rx.totalParts)
	idx = rx.lookupMapHash(mh, windowStart, windowEnd)
	if idx < 0 {
		for i := windowStart; i < windowEnd; i++ {
			if rx.partSlots[i] != nil {
				continue
			}
			if len(rx.mapHashes[i]) != resource.MapHashLen {
				continue
			}
			if bytes.Equal(rx.mapHashes[i], mh) {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		// Fall back to a collision-guard range so slightly out-of-window
		// retransmits from a prior request can still land.
		guardEnd := min(windowStart+resource.CollisionGuardSize, rx.totalParts)
		idx = rx.lookupMapHash(mh, windowStart, guardEnd)
		if idx < 0 {
			for i := windowStart; i < guardEnd; i++ {
				if rx.partSlots[i] != nil {
					continue
				}
				if len(rx.mapHashes[i]) != resource.MapHashLen {
					continue
				}
				if bytes.Equal(rx.mapHashes[i], mh) {
					idx = i
					break
				}
			}
		}
	}
	if idx < 0 {
		for i := 0; i < rx.totalParts; i++ {
			if rx.partSlots[i] == nil || len(rx.mapHashes[i]) != resource.MapHashLen {
				continue
			}
			if bytes.Equal(rx.mapHashes[i], mh) {
				debug.Log(
					debug.DebugVerbose,
					"Incoming resource duplicate part ignored",
					"link_id",
					fmt.Sprintf("%x", l.linkID),
					"part_index",
					i,
					"part_len",
					len(data),
				)
				l.incomingMu.Unlock()
				return nil
			}
		}
		debug.Log(
			debug.DebugInfo,
			"Incoming resource part map hash mismatch",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"part_len",
			len(data),
			"map_hash",
			fmt.Sprintf("%x", mh),
		)
		l.incomingMu.Unlock()
		return errors.New("incoming resource part map hash mismatch")
	}
	rx.partSlots[idx] = cloneIncomingPart(data)
	rx.lastProgressAt = time.Now()
	rx.consecutiveStalls = 0
	rx.partsReceived++
	rx.bytesReceived += int64(len(data))
	if idx >= 0 && idx < len(rx.inflight) && rx.inflight[idx] {
		rx.inflight[idx] = false
	}
	if rx.outstandingParts > 0 {
		rx.outstandingParts--
	}

	rx.consecutiveCompleted = consecutivePrefix(rx.partSlots)
	debug.Log(
		debug.DebugVerbose,
		"Incoming resource part accepted",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"part_index",
		idx,
		"consecutive_completed",
		rx.consecutiveCompleted,
		"outstanding_parts",
		rx.outstandingParts,
	)
	l.reportIncomingResourceProgress(rx)

	if l.incomingTransferComplete(rx) {
		inner := l.concatIncomingParts(rx)
		adv := rx.adv
		l.flushIncomingResourceStats(rx, "complete")
		l.incomingRx = nil
		l.incomingMu.Unlock()
		return l.deliverIncomingResource(inner, adv)
	}

	// Prefetch next HMU segment while current parts are still draining.
	prefetchHMU := false
	if !rx.waitingForHmu && rx.hashmapHeight < rx.totalParts {
		threshold := max(rx.window/2, 1)
		if rx.knownHashesAhead() < threshold {
			prefetchHMU = true
		}
	}

	win = rx.window
	if win <= 0 {
		win = resource.Window
	}
	needRefill := false
	if !rx.waitingForHmu && (rx.outstandingParts == 0 || rx.outstandingParts <= win/2) {
		if rx.outstandingParts == 0 {
			growIncomingResourceWindow(rx)
		}
		needRefill = true
	}
	l.incomingMu.Unlock()

	if prefetchHMU {
		l.queueIncomingResourceHMUPrefetch()
	}
	if needRefill {
		return l.queueIncomingResourceReqNext()
	}
	return nil
}

// growIncomingResourceWindow advances the receive window toward windowMax
// after a completed outstanding set, matching Python Resource.request_next
// pacing (WINDOW -> WINDOW_MAX_FAST on healthy paths).
func growIncomingResourceWindow(rx *incomingResourceAsm) {
	if rx == nil {
		return
	}
	if rx.window < rx.windowMax {
		step := max(rx.window/4, 1)
		rx.window += step
		if rx.window > rx.windowMax {
			rx.window = rx.windowMax
		}
		if (rx.window - rx.windowMin) > (resource.WindowFlexibility - 1) {
			rx.windowMin++
		}
	}
}

func consecutivePrefix(slots [][]byte) int {
	h := -1
	for i := range slots {
		if len(slots[i]) == 0 {
			break
		}
		h = i
	}
	return h
}

// reportIncomingResourceProgress updates the bytes-received counter on the
// pending request receipt (if this incoming resource is a response to a
// Link.Request call) so callers can surface download progress/speed/ETA
// while a large multi-part transfer is still in flight. Must be called with
// l.incomingMu held.
func (l *Link) reportIncomingResourceProgress(rx *incomingResourceAsm) {
	pending := l.incomingResourceRequest
	if pending == nil {
		return
	}
	var received int64
	for i := 0; i < rx.totalParts; i++ {
		received += int64(len(rx.partSlots[i]))
	}
	pending.mutex.Lock()
	pending.bytesReceived = received
	pending.mutex.Unlock()
}

func (l *Link) incomingTransferComplete(rx *incomingResourceAsm) bool {
	var sum int64
	for i := 0; i < rx.totalParts; i++ {
		if rx.partSlots[i] == nil {
			return false
		}
		sum += int64(len(rx.partSlots[i]))
	}
	return sum == rx.adv.TransferSize
}

func (l *Link) concatIncomingParts(rx *incomingResourceAsm) []byte {
	total := 0
	for i := 0; i < rx.totalParts; i++ {
		total += len(rx.partSlots[i])
	}
	b := make([]byte, 0, total)
	for i := 0; i < rx.totalParts; i++ {
		b = append(b, rx.partSlots[i]...)
		releaseIncomingPart(rx.partSlots[i])
		rx.partSlots[i] = nil
	}
	return b
}

func (l *Link) deliverIncomingResource(inner []byte, adv *resource.ResourceAdvertisement) error {
	payload, err := l.assembleIncomingPayload(inner, adv)
	if err != nil {
		return err
	}
	debug.Log(
		debug.DebugInfo,
		"Incoming resource assembled",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"inner_len",
		len(inner),
		"payload_len",
		len(payload),
	)
	if err := l.sendIncomingResourceProof(payload, adv.Hash); err != nil {
		return err
	}

	if adv.Split && adv.TotalSegments > 1 {
		return l.handleSplitSegmentComplete(payload, adv)
	}

	if adv.IsRequest {
		requestID := identity.TruncatedHash(payload)
		debug.Log(debug.DebugInfo, "Incoming request resource complete", "request_id", fmt.Sprintf("%x", requestID), "payload_len", len(payload))
		return l.handleRequest(payload, requestID)
	}

	l.incomingMu.Lock()
	pending := l.incomingResourceRequest
	l.incomingResourceRequest = nil
	l.incomingMu.Unlock()

	if pending != nil {
		responsePayload, metadata := splitResourceMetadata(payload, adv)
		l.completeRequestWithResourcePayload(pending, responsePayload, metadata)
		return nil
	}

	filePayload, metadata := splitResourceMetadata(payload, adv)
	if l.resourceConcludedCallback != nil {
		if metadata != nil {
			l.resourceConcludedCallback(IncomingResource{
				Data:     filePayload,
				Metadata: metadata,
				Hash:     append([]byte(nil), adv.Hash...),
			})
		} else {
			l.resourceConcludedCallback(filePayload)
		}
	}
	return nil
}

func splitResourceMetadata(payload []byte, adv *resource.ResourceAdvertisement) ([]byte, map[string]any) {
	if adv == nil || !adv.HasMetadata {
		return payload, nil
	}
	if len(payload) < 3 {
		debug.Log(debug.DebugInfo, "Incoming resource metadata flagged but payload too short")
		return payload, nil
	}
	metaSize := int(payload[0])<<16 | int(payload[1])<<8 | int(payload[2])
	if metaSize < 0 || 3+metaSize > len(payload) {
		debug.Log(debug.DebugInfo, "Incoming resource metadata size invalid", "meta_size", metaSize, "payload_len", len(payload))
		return payload, nil
	}
	// Always strip a valid length-prefixed metadata region, matching Python
	// Resource assembly. A failed msgpack unpack must not leave the 3-byte
	// size prefix and packed meta attached to the file body.
	body := payload[3+metaSize:]
	var meta map[string]any
	if err := msgpack.Unmarshal(payload[3:3+metaSize], &meta); err != nil {
		debug.Log(debug.DebugInfo, "Failed to unpack incoming resource metadata", "error", err)
		return body, nil
	}
	return body, meta
}

func (l *Link) completeRequestWithResourcePayload(req *RequestReceipt, payload []byte, metadata map[string]any) {
	respBytes := payload
	var responseValue any = payload
	if metadata == nil {
		var unpacked []any
		if err := msgpack.Unmarshal(payload, &unpacked); err == nil && len(unpacked) >= 2 {
			responseValue = unpacked[1]
			if rawResp, ok := unpacked[1].([]byte); ok {
				respBytes = rawResp
			} else if str, ok := unpacked[1].(string); ok {
				respBytes = []byte(str)
			} else if reMarshaled, err := msgpack.Marshal(unpacked[1]); err == nil {
				respBytes = reMarshaled
			}
		}
	}

	req.mutex.Lock()
	req.status = StatusActive
	req.response = respBytes
	req.responseValue = responseValue
	req.metadata = metadata
	req.receivedAt = time.Now()
	req.mutex.Unlock()

	l.requestMutex.Lock()
	for i, pending := range l.pendingRequests {
		if pending == req {
			l.pendingRequests = append(l.pendingRequests[:i], l.pendingRequests[i+1:]...)
			break
		}
	}
	l.requestMutex.Unlock()

	if req.responseCb != nil {
		go req.responseCb(req)
	}
}

func (l *Link) assembleIncomingPayload(inner []byte, adv *resource.ResourceAdvertisement) ([]byte, error) {
	var innerPlain []byte
	if adv.Encrypted {
		p, err := l.decrypt(inner)
		if err != nil {
			return nil, err
		}
		innerPlain = p
	} else {
		innerPlain = inner
	}

	if len(innerPlain) < resource.RandomHashSize {
		return nil, errors.New("incoming resource too short for random hash")
	}
	data := innerPlain[resource.RandomHashSize:]

	if adv.Compressed {
		if adv.DataSize <= 0 {
			return nil, errors.New("incoming compressed resource has invalid data_size")
		}
		if adv.DataSize > int64(resource.AutoCompressMaxSize) {
			return nil, errors.New("incoming compressed resource exceeds AutoCompressMaxSize")
		}

		r := bzip2.NewReader(bytes.NewReader(data))
		limited := io.LimitReader(r, adv.DataSize+1)
		decompressed, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if int64(len(decompressed)) > adv.DataSize {
			return nil, errors.New("incoming compressed resource exceeds advertised data_size")
		}
		data = decompressed
	}

	h := sha256.New()
	h.Write(data)
	h.Write(adv.RandomHash)
	sum := h.Sum(nil)
	if len(adv.Hash) != len(sum) || !bytes.Equal(sum, adv.Hash) {
		return nil, errors.New("incoming resource hash mismatch")
	}

	return data, nil
}

// AssembleIncomingResourcePayload decompresses and validates an assembled
// resource inner blob. Live security tests use this to exercise bz2 bomb
// rejection without a full multi-part transfer.
func AssembleIncomingResourcePayload(l *Link, inner []byte, adv *resource.ResourceAdvertisement) ([]byte, error) {
	if l == nil {
		l = &Link{}
	}
	return l.assembleIncomingPayload(inner, adv)
}

func wireInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		if x > int64(math.MaxInt) || x < int64(math.MinInt) {
			return 0, false
		}
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		if int64(x) > int64(math.MaxInt) {
			return 0, false
		}
		return int(x), true
	case uint64:
		if x > uint64(math.MaxInt) {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}
