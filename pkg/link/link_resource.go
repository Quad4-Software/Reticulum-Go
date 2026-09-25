// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func (l *Link) SetResourceCallback(callback func(any) bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceCallback = callback
}
func (l *Link) SetResourceStartedCallback(callback func(any)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceStartedCallback = callback
}
func (l *Link) SetResourceConcludedCallback(callback func(any)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceConcludedCallback = callback
}
func (l *Link) SetResourceStrategy(strategy byte) error {
	if strategy != AcceptNone && strategy != AcceptAll && strategy != AcceptApp {
		return errors.New("unsupported resource strategy")
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceStrategy = strategy
	return nil
}
func (l *Link) signalOutgoingResourceComplete() {
	l.outgoingMu.Lock()
	ch := l.outgoingResCompleteChan
	l.outgoingResCompleteChan = nil
	l.outgoingRes = nil
	l.outgoingReceiverMinPart = 0
	l.outgoingMu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
func (l *Link) handleResourceProof(pkt *packet.Packet) error {
	if len(pkt.Data) < sha256.Size*2 {
		return nil
	}
	resourceHash := pkt.Data[:sha256.Size]
	receivedProof := pkt.Data[sha256.Size : sha256.Size*2]

	l.outgoingMu.Lock()
	out := l.outgoingRes
	l.outgoingMu.Unlock()
	if out == nil {
		return nil
	}
	if !bytes.Equal(out.GetHash(), resourceHash) {
		return nil
	}
	expectedProof, ok := out.ExpectedProof()
	if !ok || !bytes.Equal(receivedProof, expectedProof) {
		debug.Log(
			debug.DebugVerbose,
			"Ignoring invalid outgoing resource proof",
			"resource_hash",
			fmt.Sprintf("%x", resourceHash),
		)
		return nil
	}

	debug.Log(debug.DebugVerbose, "Outgoing resource proof received", "resource_hash", fmt.Sprintf("%x", resourceHash))
	if out.IsSplit() && out.GetSegmentIndex() < out.GetTotalSegments() {
		if err := out.PrepareNextOutboundSegment(l.encrypt, l.resourceSDU()); err != nil {
			l.signalOutgoingResourceComplete()
			return fmt.Errorf("prepare next resource segment: %w", err)
		}
		out.Activate()
		l.outgoingMu.Lock()
		l.outgoingRes = out
		l.outgoingReceiverMinPart = 0
		l.outgoingMu.Unlock()
		if err := l.sendResourceAdvertisement(out); err != nil {
			l.signalOutgoingResourceComplete()
			return fmt.Errorf("advertise next resource segment: %w", err)
		}
		return nil
	}
	l.signalOutgoingResourceComplete()
	return nil
}
func (l *Link) handleResourceAdvertisement(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}
	if err := l.processResourceAdvertisement(plaintext); err != nil {
		return l.abortInvalidResourceAdvertisement(err)
	}
	return nil
}

// abortInvalidResourceAdvertisement tears the link down after a bad RESOURCE_ADV.
func (l *Link) abortInvalidResourceAdvertisement(err error) error {
	debug.Log(debug.DebugWarning, "Invalid resource advertisement", "error", err)
	l.Teardown()
	return err
}

// processResourceAdvertisement accepts or rejects a decrypted RESOURCE_ADV.
// Malformed or oversized advertisements return an error so the caller can
// close the link.
func (l *Link) processResourceAdvertisement(plaintext []byte) error {
	adv, err := resource.UnpackResourceAdvertisement(plaintext)
	if err != nil {
		return err
	}

	if adv.Split {
		debug.Log(debug.DebugVerbose, "Accepting split resource advertisement",
			"hash", fmt.Sprintf("%x", adv.Hash),
			"original", fmt.Sprintf("%x", adv.OriginalHash),
			"segment", adv.SegmentIndex,
			"total", adv.TotalSegments)
	}

	if adv.IsRequest && adv.RequestID != nil {
		if !l.destination.HasRequestHandlers() {
			debug.Log(debug.DebugVerbose, "Ignoring request resource advertisement")
			return nil
		}
		if maxSize, ok := l.destination.MaxRequestSize(); ok && int(adv.TransferSize) > maxSize {
			debug.Log(debug.DebugVerbose, "Ignored request resource with excessive size",
				"bytes", adv.TransferSize, "max", maxSize)
			return nil
		}
		return l.beginIncomingResource(adv)
	}

	if adv.IsResponse && adv.RequestID != nil {
		requestID := adv.RequestID
		var matched *RequestReceipt
		l.requestMutex.RLock()
		for _, req := range l.pendingRequests {
			if string(req.requestID) == string(requestID) {
				matched = req
				break
			}
		}
		l.requestMutex.RUnlock()

		if matched == nil {
			debug.Log(debug.DebugVerbose, "Received response resource advertisement for unknown request", "request_id", fmt.Sprintf("%x", requestID))
			return nil
		}
		if matched.maxResponseSize > 0 && int(adv.TransferSize) > matched.maxResponseSize {
			debug.Log(debug.DebugVerbose, "Rejected response resource with excessive size",
				"bytes", adv.TransferSize, "max", matched.maxResponseSize)
			l.failPendingRequest(matched)
			return nil
		}

		matched.mutex.Lock()
		matched.totalBytes = adv.TransferSize
		matched.mutex.Unlock()

		if err := l.beginIncomingResource(adv); err != nil {
			return err
		}
		l.bindIncomingResponseRequest(adv, matched)
		return nil
	}

	if l.resourceStrategy == AcceptNone {
		_ = l.rejectResource(adv.Hash) // #nosec G104 - best effort resource rejection
		debug.Log(debug.DebugVerbose, "Resource advertisement rejected (AcceptNone)")
		return nil
	}

	allowed := false
	if l.resourceStrategy == AcceptAll {
		allowed = true
	} else if l.resourceStrategy == AcceptApp && l.resourceCallback != nil {
		allowed = l.resourceCallback(adv)
	}

	if allowed {
		if err := l.beginIncomingResource(adv); err != nil {
			return err
		}
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(adv)
		}
	} else {
		_ = l.rejectResource(adv.Hash) // #nosec G104 - best effort resource rejection
		debug.Log(debug.DebugVerbose, "Resource advertisement rejected")
	}

	return nil
}

// sendIncomingResourceProof notifies the sender that the resource was assembled correctly
// (SHA-256(payload||resourceHash)), matching Resource.prove / validate_proof.
func (l *Link) sendIncomingResourceProof(payload []byte, resourceHash []byte) error {
	if len(resourceHash) != sha256.Size {
		return errors.New("resource hash must be 32 bytes")
	}
	sum := sha256.Sum256(append(append([]byte(nil), payload...), resourceHash...))
	proofData := append(append([]byte(nil), resourceHash...), sum[:]...)
	proofPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeProof,
		TransportType:   0,
		Context:         packet.ContextResourcePRF,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            proofData,
		CreateReceipt:   false,
	}
	if err := proofPkt.Pack(); err != nil {
		return err
	}
	l.recordOutbound()
	return l.transport.SendPacket(proofPkt)
}
func (l *Link) rejectResource(resourceHash []byte) error {
	rejectPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextResourceRCL,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            resourceHash,
		CreateReceipt:   false,
	}
	encrypted, err := l.encrypt(resourceHash)
	if err != nil {
		return err
	}
	rejectPkt.Data = encrypted
	if err := rejectPkt.Pack(); err != nil {
		return err
	}
	l.recordOutbound()
	return l.transport.SendPacket(rejectPkt)
}
func (l *Link) sendResourceResponse(requestID []byte, response any) error {
	return l.sendResponse(requestID, response)
}
func (l *Link) sendResourceAdvertisement(res *resource.Resource) error {
	adv := resource.NewResourceAdvertisement(res)
	if adv == nil {
		return errors.New("failed to create resource advertisement")
	}

	l.mutex.RLock()
	mdu := l.mdu
	l.mutex.RUnlock()

	advData, err := adv.Pack(0, mdu)
	if err != nil {
		return fmt.Errorf("failed to pack advertisement: %w", err)
	}

	encrypted, err := l.encrypt(advData)
	if err != nil {
		return err
	}

	advPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextResourceAdv,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            encrypted,
		CreateReceipt:   false,
	}

	if err := advPkt.Pack(); err != nil {
		return err
	}

	l.recordOutbound()
	return l.transport.SendPacket(advPkt)
}
func (l *Link) dispatchOutgoingResourceRequests(plaintext []byte) {
	l.outgoingDispatchMu.Lock()
	defer l.outgoingDispatchMu.Unlock()

	l.outgoingMu.Lock()
	out := l.outgoingRes
	receiverMinPart := l.outgoingReceiverMinPart
	l.outgoingMu.Unlock()
	if out == nil {
		debug.Log(debug.DebugVerbose, "Ignoring resource request: no outgoing resource")
		return
	}
	if len(plaintext) < 1+32 {
		debug.Log(debug.DebugVerbose, "Ignoring resource request: payload too short", "len", len(plaintext))
		return
	}
	var resourceHash []byte
	var hmuAnchorHash []byte
	var pad int
	if plaintext[0] == LinkResourceMappedFlag {
		pad = 1 + resource.MapHashLen
		if len(plaintext) < pad+32 {
			debug.Log(debug.DebugVerbose, "Ignoring mapped resource request: payload too short", "len", len(plaintext))
			return
		}
		hmuAnchorHash = plaintext[1:pad]
		resourceHash = plaintext[pad : pad+32]
	} else {
		pad = 1
		resourceHash = plaintext[pad : pad+32]
	}
	if !bytes.Equal(resourceHash, out.GetHash()) {
		debug.Log(
			debug.DebugVerbose,
			"Ignoring resource request: hash mismatch",
			"request_hash",
			fmt.Sprintf("%x", resourceHash),
			"out_hash",
			fmt.Sprintf("%x", out.GetHash()),
		)
		return
	}
	reqHashes := plaintext[pad+32:]
	if len(reqHashes)%resource.MapHashLen != 0 {
		debug.Log(debug.DebugVerbose, "Ignoring resource request: invalid hash vector length", "len", len(reqHashes))
		return
	}
	l.mutex.RLock()
	hashmapMDU := l.mdu
	l.mutex.RUnlock()
	partSDU := l.resourceSDU()
	// Select and send parts for the current receiver window first, then
	// advance the window and emit HMU. Updating receiverMinPart before
	// selection drops in-window hashes from the same HASHMAP_IS_EXHAUSTED
	// request and stalls multi-HMU transfers.
	partIndexes := selectRequestedPartIndexes(out, reqHashes, receiverMinPart)
	debug.Log(
		debug.DebugVerbose,
		"Outgoing resource part request selection",
		"resource_hash",
		fmt.Sprintf("%x", resourceHash),
		"requested_hashes",
		len(reqHashes)/resource.MapHashLen,
		"selected_parts",
		len(partIndexes),
		"receiver_min_part",
		receiverMinPart,
	)
	if len(reqHashes) > 0 && len(partIndexes) == 0 {
		debug.Log(
			debug.DebugVerbose,
			"Outgoing resource request matched no parts",
			"resource_hash",
			fmt.Sprintf("%x", resourceHash),
			"requested_hashes",
			len(reqHashes)/resource.MapHashLen,
			"receiver_min_part",
			receiverMinPart,
			"hmu_request",
			len(hmuAnchorHash) == resource.MapHashLen,
		)
	}
	partBuf := make([]byte, 0, partSDU)
	for _, pi := range partIndexes {
		slice := out.OutboundCiphertextSliceInto(partBuf, pi, partSDU)
		if len(slice) == 0 {
			continue
		}
		partBuf = slice
		if err := l.SendPacketWithContext(slice, packet.ContextResource); err != nil {
			return
		}
		_ = out.MarkOutboundPartSent(pi)
	}
	if len(hmuAnchorHash) == resource.MapHashLen {
		debug.Log(
			debug.DebugVerbose,
			"Outgoing resource received HMU request",
			"resource_hash",
			fmt.Sprintf("%x", resourceHash),
			"anchor_hash",
			fmt.Sprintf("%x", hmuAnchorHash),
			"receiver_min_part",
			receiverMinPart,
		)
		nextMin, err := l.sendResourceHashmapUpdate(out, hashmapMDU, hmuAnchorHash, receiverMinPart)
		if err == nil && nextMin >= 0 {
			l.outgoingMu.Lock()
			if l.outgoingRes == out {
				l.outgoingReceiverMinPart = nextMin
			}
			l.outgoingMu.Unlock()
		}
	}
}
func chooseHashmapUpdateSegment(out *resource.Resource, sdu int, anchorHash []byte, receiverMinPart int) (segment int, nextMin int, ok bool) {
	if out == nil || len(anchorHash) != resource.MapHashLen {
		return 0, 0, false
	}
	entries := resource.HashmapEntriesPerSegment(sdu)
	if entries <= 0 {
		entries = 1
	}
	totalParts := int(out.GetSegments())
	if totalParts == 0 {
		return 0, 0, false
	}
	if receiverMinPart < 0 {
		receiverMinPart = 0
	}
	// Look back one hashmap segment so a lost HMU can be resent after
	// outgoingReceiverMinPart was already advanced on the prior send.
	// Without this, the receiver stalls forever re-requesting an HMU whose
	// anchor now sits below receiverMinPart.
	searchStart := max(receiverMinPart-entries, 0)
	if searchStart >= totalParts {
		searchStart = 0
	}
	searchEnd := min(receiverMinPart+resource.CollisionGuardSize, totalParts)
	if searchEnd < searchStart+entries {
		searchEnd = min(searchStart+resource.CollisionGuardSize, totalParts)
	}

	target := -1
	fallback := -1
	for idx := searchStart; idx < searchEnd; idx++ {
		mh := out.MapHashAt(idx)
		if len(mh) != resource.MapHashLen {
			continue
		}
		if bytes.Equal(mh, anchorHash) {
			if fallback < 0 {
				fallback = idx
			}
			if idx+1 < totalParts && (idx+1)%entries == 0 {
				target = idx
				break
			}
		}
	}
	if target < 0 {
		target = fallback
	}
	if target < 0 {
		return 0, 0, false
	}

	segment = (target + 1) / entries
	if segment <= 0 {
		return 0, 0, false
	}
	nextMin = target + 1
	return segment, nextMin, true
}
func (l *Link) sendResourceHashmapUpdate(out *resource.Resource, sdu int, anchorHash []byte, receiverMinPart int) (int, error) {
	segment, nextMin, ok := chooseHashmapUpdateSegment(out, sdu, anchorHash, receiverMinPart)
	if !ok {
		return -1, nil
	}
	hashmap := out.HashmapSegment(sdu, segment)
	if len(hashmap) == 0 {
		// Match Python Resource.request_hashed_data_maps (RNS 1.3.9).
		debug.Log(debug.DebugError, "Resource HMU error, cancelling transfer",
			"resource_hash", fmt.Sprintf("%x", out.GetHash()))
		out.Cancel()
		l.signalOutgoingResourceComplete()
		return -1, errors.New("empty hashmap update")
	}
	update, err := msgpack.Marshal([]any{segment, hashmap})
	if err != nil {
		return -1, err
	}
	payload := append(append([]byte{}, out.GetHash()...), update...)
	if err := l.SendPacketWithContext(payload, packet.ContextResourceHMU); err != nil {
		return -1, err
	}
	debug.Log(
		debug.DebugVerbose,
		"Outgoing HMU sent",
		"resource_hash",
		fmt.Sprintf("%x", out.GetHash()),
		"segment",
		segment,
		"entries",
		len(hashmap)/resource.MapHashLen,
		"next_min_part",
		nextMin,
	)
	return nextMin, nil
}
func selectRequestedPartIndexes(out *resource.Resource, reqHashes []byte, receiverMinPart int) []int {
	if out == nil || len(reqHashes)%resource.MapHashLen != 0 {
		return nil
	}
	totalParts := int(out.GetSegments())
	if totalParts == 0 {
		return nil
	}
	if receiverMinPart < 0 {
		receiverMinPart = 0
	}
	searchStart := receiverMinPart
	if searchStart >= totalParts {
		searchStart = 0
	}
	searchEnd := min(searchStart+resource.CollisionGuardSize, totalParts)

	// Restrict map-hash lookup to parts[receiverMin:receiverMin+CollisionGuard].
	// Global PartIndicesForMapHash fallbacks can retransmit earlier parts whose
	// map hashes collide with the request window. Those late duplicates can
	// reset a peer that has already assembled and proved back to TRANSFERRING.
	usedPartIndexes := make(map[int]struct{})
	indexes := make([]int, 0, len(reqHashes)/resource.MapHashLen)
	for i := 0; i < len(reqHashes); i += resource.MapHashLen {
		mh := reqHashes[i : i+resource.MapHashLen]
		pi := -1
		for idx := searchStart; idx < searchEnd; idx++ {
			if _, used := usedPartIndexes[idx]; used {
				continue
			}
			mapHash := out.MapHashAt(idx)
			if len(mapHash) != resource.MapHashLen || !bytes.Equal(mapHash, mh) {
				continue
			}
			if !out.IsOutboundPartSent(idx) {
				pi = idx
				break
			}
		}
		if pi < 0 {
			for idx := searchStart; idx < searchEnd; idx++ {
				if _, used := usedPartIndexes[idx]; used {
					continue
				}
				mapHash := out.MapHashAt(idx)
				if len(mapHash) == resource.MapHashLen && bytes.Equal(mapHash, mh) {
					pi = idx
					break
				}
			}
		}
		// If the receiver is still asking for hashes that sit just below
		// receiverMinPart (lost HMU / partial window), allow a one-guard
		// lookback so the transfer can recover instead of matching nothing.
		if pi < 0 && receiverMinPart > 0 {
			backStart := max(receiverMinPart-resource.CollisionGuardSize, 0)
			for idx := backStart; idx < searchStart; idx++ {
				if _, used := usedPartIndexes[idx]; used {
					continue
				}
				mapHash := out.MapHashAt(idx)
				if len(mapHash) == resource.MapHashLen && bytes.Equal(mapHash, mh) {
					pi = idx
					break
				}
			}
		}
		if pi < 0 {
			continue
		}

		usedPartIndexes[pi] = struct{}{}
		indexes = append(indexes, pi)
	}
	return indexes
}
func (l *Link) handleResourceRequest(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	l.outgoingMu.Lock()
	out := l.outgoingRes
	l.outgoingMu.Unlock()
	if out != nil && len(plaintext) >= 1+32 {
		l.dispatchOutgoingResourceRequests(plaintext)
		return nil
	}

	if l.resourceStartedCallback != nil {
		l.resourceStartedCallback(plaintext)
	}

	return nil
}
func (l *Link) handleResourceHashmapUpdate(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	if len(plaintext) < sha256.Size {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(plaintext)
		}
		return nil
	}

	resHash := plaintext[:sha256.Size]
	var update []any
	if err := msgpack.Unmarshal(plaintext[sha256.Size:], &update); err != nil {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(plaintext)
		}
		return nil
	}
	if len(update) < 2 {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(plaintext)
		}
		return nil
	}
	seg, ok := wireInt(update[0])
	if !ok {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(plaintext)
		}
		return nil
	}
	hm, ok := update[1].([]byte)
	if !ok {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(plaintext)
		}
		return nil
	}

	if err := l.applyIncomingHashmapUpdate(resHash, seg, hm); err != nil {
		return err
	}

	if l.resourceStartedCallback != nil {
		l.resourceStartedCallback(plaintext)
	}

	return nil
}
func (l *Link) handleResourceCancel(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}
	l.resetIncomingResource()
	// Match Python 1.3.9 resource.cancel on the receiver: notify the initiator
	// with RESOURCE_RCL after cancelling the incoming transfer.
	if l.status.Load() == int32(StatusActive) && len(plaintext) >= sha256.Size {
		_ = l.rejectResource(plaintext[:sha256.Size]) // #nosec G104 - best effort
	}
	return nil
}
func (l *Link) handleResourceReject(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}
	if len(plaintext) < sha256.Size {
		return nil
	}
	resourceHash := plaintext[:sha256.Size]
	l.outgoingMu.Lock()
	out := l.outgoingRes
	l.outgoingMu.Unlock()
	if out == nil || !bytes.Equal(out.GetHash(), resourceHash) {
		return nil
	}
	out.Cancel()
	l.signalOutgoingResourceComplete()
	return nil
}
func (l *Link) handleResourcePart(data []byte, pkt *packet.Packet) error {
	l.incomingMu.Lock()
	hasAsm := l.incomingRx != nil
	l.incomingMu.Unlock()
	if hasAsm {
		return l.appendIncomingResourcePart(data)
	}
	if len(data) == 0 {
		return nil
	}
	if l.resourceStartedCallback != nil {
		l.resourceStartedCallback(data)
	}

	return nil
}

// acquireResourceSendSlot reserves one of MaxPendingResourceSends slots for a
// goroutine blocked inside SendResource.
func (l *Link) acquireResourceSendSlot() bool {
	for {
		n := l.resSendInflight.Load()
		if n >= MaxPendingResourceSends {
			return false
		}
		if l.resSendInflight.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

func (l *Link) releaseResourceSendSlot() {
	l.resSendInflight.Add(-1)
}

func (l *Link) SendResource(res *resource.Resource) error {
	l.resourceSendMu.Lock()
	defer l.resourceSendMu.Unlock()

	l.mutex.Lock()
	if l.status.Load() != int32(StatusActive) {
		l.teardownReason = StatusFailed
		l.mutex.Unlock()
		return common.ErrLinkNotActive
	}
	l.mutex.Unlock()
	sdu := l.resourceSDU()

	if err := res.PrepareOutboundForLink(l.encrypt, sdu); err != nil {
		return err
	}

	l.mutex.Lock()
	res.Activate()
	l.mutex.Unlock()

	done := make(chan struct{}, 1)
	l.outgoingMu.Lock()
	l.outgoingRes = res
	l.outgoingReceiverMinPart = 0
	l.outgoingResCompleteChan = done
	l.outgoingMu.Unlock()

	if err := l.sendResourceAdvertisement(res); err != nil {
		l.outgoingMu.Lock()
		l.outgoingRes = nil
		l.outgoingReceiverMinPart = 0
		l.outgoingResCompleteChan = nil
		l.outgoingMu.Unlock()
		l.mutex.Lock()
		l.teardownReason = StatusFailed
		l.mutex.Unlock()
		return fmt.Errorf("resource advertisement: %w", err)
	}

	if res.GetSegments() == 0 {
		if err := l.SendPacketWithContext(nil, packet.ContextResource); err != nil {
			l.outgoingMu.Lock()
			l.outgoingRes = nil
			l.outgoingReceiverMinPart = 0
			l.outgoingResCompleteChan = nil
			l.outgoingMu.Unlock()
			return err
		}
		l.signalOutgoingResourceComplete()
		return nil
	}

	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Minute):
		l.outgoingMu.Lock()
		l.outgoingRes = nil
		l.outgoingReceiverMinPart = 0
		l.outgoingResCompleteChan = nil
		l.outgoingMu.Unlock()
		l.mutex.Lock()
		l.teardownReason = StatusFailed
		l.mutex.Unlock()
		return errors.New("resource transfer timeout")
	}
}
