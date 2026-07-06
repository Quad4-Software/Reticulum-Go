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
	"time"

	"quad4/bzip2/pkg/bzip2"
	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/resource"
)

const (
	hashmapNotExhausted = 0x00
	hashmapExhausted    = 0xff

	// incomingResourceRetryInterval controls how often the receive-side
	// watchdog nudges a stalled transfer by re-sending the current part (or
	// HMU) request. sendIncomingResourceReqNext is otherwise only driven by
	// arriving packets, so if a request packet or every part in the
	// requested window is lost in transit -- routine on lossy, multi-hop
	// mesh paths -- the receiver would otherwise wait silently until the
	// much longer outer Link.Request timeout gives up on the whole
	// transfer.
	incomingResourceRetryInterval = 4 * time.Second
)

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
	hashmapHeight        int
	totalParts           int
	consecutiveCompleted int
	waitingForHmu        bool
	lastProgressAt       time.Time
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
		rx.mapHashes[idx] = append([]byte(nil), hashmapBytes[off:off+resource.MapHashLen]...)
	}
	return added
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
	if adv.Parts > int(resource.MaxSegments) {
		return errors.New("incoming resource parts exceed MaxSegments")
	}
	if adv.TransferSize < 0 {
		return errors.New("incoming resource has negative transfer_size")
	}
	maxTransfer := int64(adv.Parts) * int64(sdu)
	if adv.TransferSize > maxTransfer {
		return errors.New("incoming resource transfer_size exceeds parts*sdu")
	}
	if len(adv.Hashmap) == 0 || len(adv.Hashmap)%resource.MapHashLen != 0 {
		return errors.New("invalid advertisement hashmap")
	}

	rx := &incomingResourceAsm{
		adv:                  adv,
		sdu:                  sdu,
		hashmapSegLen:        hashmapSegLen,
		partSlots:            make([][]byte, adv.Parts),
		mapHashes:            make([][]byte, adv.Parts),
		totalParts:           adv.Parts,
		consecutiveCompleted: -1,
		lastProgressAt:       time.Now(),
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
	return l.sendIncomingResourceReqNext()
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
	if idle < incomingResourceRetryInterval {
		l.incomingMu.Unlock()
		return true
	}
	stalledOnHmu := rx.waitingForHmu
	rx.waitingForHmu = false
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
	)
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

	end := min(searchStart+resource.Window, rx.totalParts)

	requestedHashes := make([]byte, 0, resource.Window*resource.MapHashLen)
	exhausted := false
	batch := 0
	for pn := searchStart; pn < end; pn++ {
		if rx.partSlots[pn] != nil {
			continue
		}
		mh := rx.mapHashes[pn]
		if mh != nil {
			requestedHashes = append(requestedHashes, mh...)
			batch++
			if batch >= resource.Window {
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
		rx.waitingForHmu = true
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

	reqBody := append(prefix, rx.adv.Hash...)
	reqBody = append(reqBody, requestedHashes...)
	debug.Log(
		debug.DebugVerbose,
		"Incoming resource requesting parts",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"search_start",
		searchStart,
		"window_end",
		end,
		"requested_parts",
		len(requestedHashes)/resource.MapHashLen,
		"waiting_for_hmu",
		exhausted,
	)
	l.incomingMu.Unlock()

	return l.SendPacketWithContext(reqBody, packet.ContextResourceReq)
}

func (l *Link) resetIncomingResource() {
	l.incomingMu.Lock()
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
	added := rx.applyHashmapSegment(segment, hashmapBytes)
	if added > 0 {
		rx.lastProgressAt = time.Now()
	}
	if added == 0 {
		debug.Log(
			debug.DebugVerbose,
			"Incoming duplicate HMU ignored",
			"link_id",
			fmt.Sprintf("%x", l.linkID),
			"segment",
			segment,
			"entries",
			len(hashmapBytes)/resource.MapHashLen,
			"hashmap_height",
			rx.hashmapHeight,
		)
		l.incomingMu.Unlock()
		return nil
	}
	rx.waitingForHmu = false
	debug.Log(
		debug.DebugVerbose,
		"Incoming HMU applied",
		"link_id",
		fmt.Sprintf("%x", l.linkID),
		"segment",
		segment,
		"entries",
		len(hashmapBytes)/resource.MapHashLen,
		"hashmap_height",
		rx.hashmapHeight,
	)
	l.incomingMu.Unlock()
	return l.sendIncomingResourceReqNext()
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
	for i := 0; i < rx.totalParts; i++ {
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
	rx.partSlots[idx] = append([]byte(nil), data...)
	rx.lastProgressAt = time.Now()

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
	)
	l.reportIncomingResourceProgress(rx)

	if l.incomingTransferComplete(rx) {
		inner := l.concatIncomingParts(rx)
		adv := rx.adv
		l.incomingRx = nil
		l.incomingMu.Unlock()
		return l.deliverIncomingResource(inner, adv)
	}

	l.incomingMu.Unlock()
	return l.sendIncomingResourceReqNext()
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

	l.incomingMu.Lock()
	pending := l.incomingResourceRequest
	l.incomingResourceRequest = nil
	l.incomingMu.Unlock()

	if pending != nil {
		responsePayload, metadata := splitResourceMetadata(payload, adv)
		l.completeRequestWithResourcePayload(pending, responsePayload, metadata)
		return nil
	}

	if l.resourceConcludedCallback != nil {
		l.resourceConcludedCallback(payload)
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
	var meta map[string]any
	if err := msgpack.Unmarshal(payload[3:3+metaSize], &meta); err != nil {
		debug.Log(debug.DebugInfo, "Failed to unpack incoming resource metadata", "error", err)
		return payload, nil
	}
	return payload[3+metaSize:], meta
}

func (l *Link) completeRequestWithResourcePayload(req *RequestReceipt, payload []byte, metadata map[string]any) {
	respBytes := payload
	if metadata == nil {
		var unpacked []any
		if err := msgpack.Unmarshal(payload, &unpacked); err == nil && len(unpacked) >= 2 {
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
