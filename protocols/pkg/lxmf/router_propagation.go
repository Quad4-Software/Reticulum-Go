// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/packet"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
)

const (
	pnStampThrottleSeconds = 180
	maxInboundSyncsDefault = 3
)

func (r *Router) propagationLinkEstablished(v any) {
	lnk, ok := v.(*link.Link)
	if !ok || lnk == nil {
		return
	}
	_ = lnk.SetResourceStrategy(link.AcceptApp)
	lnk.SetPacketCallback(func(data []byte, pkt *packet.Packet) {
		r.handlePropagationPacket(data, pkt, lnk)
	})
	lnk.SetResourceCallback(func(adv any) bool {
		return r.propagationResourceAdvertised(adv, lnk)
	})
	lnk.SetResourceConcludedCallback(func(res any) {
		r.propagationResourceConcluded(res, lnk)
	})
	linkKey := hex.EncodeToString(lnk.GetLinkID())
	r.linksMu.Lock()
	r.propLinks[linkKey] = lnk
	r.linksMu.Unlock()
	// Dropping inbound sync link accounting on close matches upstream
	// clean_links, which clears validated_peer_links on teardown.
	lnk.SetLinkClosedCallback(func(closed *link.Link) {
		closedKey := hex.EncodeToString(closed.GetLinkID())
		r.propMu.Lock()
		delete(r.validatedPeerLinks, closedKey)
		delete(r.acceptedOfferLinks, closedKey)
		r.propMu.Unlock()
		r.linksMu.Lock()
		delete(r.propLinks, closedKey)
		r.linksMu.Unlock()
	})
}

func (r *Router) handlePropagationPacket(data []byte, pkt *packet.Packet, lnk *link.Link) {
	defer func() {
		if rec := recover(); rec != nil {
			Error("propagation packet panic", "error", rec)
		}
	}()

	var payload []any
	if err := msgpack.Unmarshal(data, &payload); err != nil || len(payload) < 2 {
		return
	}
	messages, ok := payload[1].([]any)
	if !ok {
		return
	}
	raw := make([][]byte, 0, len(messages))
	for _, m := range messages {
		b, ok := m.([]byte)
		if ok && len(b) > 0 {
			raw = append(raw, b)
		}
	}

	minCost := max(0, r.cfg.Propagation.PropagationStampCostTarget-r.cfg.Propagation.PropagationStampCostFlexibility)
	validated := ValidatePNStamps(raw, minCost)

	for _, entry := range validated {
		r.lxmfPropagation(entry.LxmData, int64(entry.Value), entry.Stamp, nil, false)
		r.clientPropagationReceived++
	}

	if len(validated) == len(raw) {
		if lnk != nil && pkt != nil {
			if err := lnk.ProvePacket(pkt); err != nil {
				Warning("propagation client packet proof failed", "error", err)
			} else {
				Verbose("propagation client packet proof sent", "messages", len(raw))
			}
		} else {
			Warning("propagation client transfer missing proof context", "has_link", lnk != nil, "has_packet", pkt != nil)
		}
		return
	}
	Warning("propagation client transfer had invalid stamps", "valid", len(validated), "total", len(raw))
	if lnk != nil {
		reject, _ := msgpack.Marshal([]any{PeerErrorInvalidStamp})
		_ = lnk.SendPacket(reject)
		lnk.Teardown()
	}
}

func (r *Router) propagationResourceAdvertised(adv any, lnk *link.Link) bool {
	if r.cfg.Propagation.FromStaticOnly {
		remote := lnk.GetRemoteIdentity()
		if remote == nil {
			return false
		}
		propHash, err := PropagationDestinationHash(remote)
		if err != nil || !r.isStaticPeer(propHash) {
			return false
		}
	}
	var size int64
	switch a := adv.(type) {
	case *resource.ResourceAdvertisement:
		size = a.DataSize
	}
	limit := int64(r.cfg.Propagation.PropagationSyncMaxAcceptedKB * 1000)
	if limit > 0 && size > limit {
		return false
	}
	linkID := hex.EncodeToString(lnk.GetLinkID())
	r.propMu.Lock()
	if state, ok := r.acceptedOfferLinks[linkID]; ok && state >= OfferAccepted {
		r.acceptedOfferLinks[linkID] = OfferTransferring
	}
	r.propMu.Unlock()
	return true
}

func (r *Router) propagationResourceConcluded(res any, lnk *link.Link) {
	linkID := hex.EncodeToString(lnk.GetLinkID())
	// Upstream pops the inbound sync accounting entry when the transfer
	// concludes, whether it completed or failed.
	defer func() {
		r.propMu.Lock()
		delete(r.acceptedOfferLinks, linkID)
		r.propMu.Unlock()
	}()

	data := extractResourceData(res)
	if len(data) == 0 {
		return
	}
	// Upstream requires exactly [timebase, messages] for sync resources.
	var payload []any
	if err := msgpack.Unmarshal(data, &payload); err != nil || len(payload) != 2 {
		return
	}
	messages, ok := payload[1].([]any)
	if !ok {
		return
	}

	remote := lnk.GetRemoteIdentity()
	var remotePropHash []byte
	if remote != nil {
		remotePropHash, _ = PropagationDestinationHash(remote)
	}
	r.tryAutopeerFromIncomingSync(remotePropHash)

	r.propMu.Lock()
	peeringValidated := r.validatedPeerLinks[linkID]
	r.propMu.Unlock()

	raw := make([][]byte, 0, len(messages))
	for _, m := range messages {
		b, ok := m.([]byte)
		if ok && len(b) > 0 {
			raw = append(raw, b)
		}
	}
	if len(raw) == 0 {
		return
	}

	if !peeringValidated && len(raw) > 1 {
		Warning("rejecting multi-message propagation without peering key", "peer", hex.EncodeToString(remotePropHash))
		lnk.Teardown()
		return
	}

	if remotePropHash != nil {
		// Upstream records the inbound sync as validating whenever the
		// remote identity is known, regardless of bypass state.
		r.propMu.Lock()
		r.acceptedOfferLinks[linkID] = OfferValidating
		r.validatingFrom[peerKey(remotePropHash)] = float64(time.Now().Unix())
		r.propMu.Unlock()
	}

	minCost := max(0, r.cfg.Propagation.PropagationStampCostTarget-r.cfg.Propagation.PropagationStampCostFlexibility)
	validated := ValidatePNStamps(raw, minCost)

	var fromPeer *Peer
	if remotePropHash != nil {
		r.peersMu.RLock()
		fromPeer = r.peers[peerKey(remotePropHash)]
		r.peersMu.RUnlock()
	}

	for _, entry := range validated {
		r.lxmfPropagation(entry.LxmData, int64(entry.Value), entry.Stamp, fromPeer, false)
		if fromPeer != nil {
			fromPeer.Incoming++
			fromPeer.RXBytes += int64(len(entry.LxmData))
			fromPeer.queueHandled(entry.TransientID)
		} else if remote != nil {
			r.unpeeredPropagationIncoming++
			r.unpeeredPropagationRxBytes += int64(len(entry.LxmData))
		} else {
			r.clientPropagationReceived++
		}
	}

	r.propMu.Lock()
	if remotePropHash != nil {
		delete(r.validatingFrom, peerKey(remotePropHash))
	}
	r.propMu.Unlock()

	invalidCount := len(raw) - len(validated)
	if invalidCount > 0 {
		Warning("propagation resource had invalid stamps", "valid", len(validated), "total", len(raw), "peer", hex.EncodeToString(remotePropHash))
		if remotePropHash != nil {
			r.throttlePeer(remotePropHash)
		}
		lnk.Teardown()
	}
}

// remoteIdentityGrace bounds how long a request handler waits for an
// in-flight identify packet to register the link's remote identity.
// Inbound packets are dispatched by parallel workers, so an identify
// sent right after link establishment can still be in flight. Upstream
// processes inbound packets serially and never observes that ordering.
const remoteIdentityGrace = 2 * time.Second

// requestRemoteIdentity resolves the remote identity for a link-bound
// request, waiting briefly when the identify has not landed yet.
func (r *Router) requestRemoteIdentity(linkID []byte, remoteIdentity *identity.Identity) *identity.Identity {
	if remoteIdentity != nil {
		return remoteIdentity
	}
	key := hex.EncodeToString(linkID)
	deadline := time.Now().Add(remoteIdentityGrace)
	for time.Now().Before(deadline) {
		r.linksMu.Lock()
		lnk := r.propLinks[key]
		if lnk == nil {
			lnk = r.directLinks[key]
		}
		if lnk == nil {
			lnk = r.controlLinks[key]
		}
		r.linksMu.Unlock()
		if lnk == nil {
			return nil
		}
		if id := lnk.GetRemoteIdentity(); id != nil {
			return id
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func (r *Router) offerRequestHandler(path string, data []byte, requestID, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	_ = path
	_ = requestID
	_ = requestedAt

	remoteIdentity = r.requestRemoteIdentity(linkID, remoteIdentity)
	if remoteIdentity == nil {
		return int(PeerErrorNoIdentity)
	}
	remotePropHash, err := PropagationDestinationHash(remoteIdentity)
	if err != nil {
		return int(PeerErrorInvalidData)
	}
	remoteKey := peerKey(remotePropHash)
	linkKey := hex.EncodeToString(linkID)

	bypassSeq := r.isStaticPeer(remotePropHash) && r.cfg.Propagation.StaticPeersBypassSequential
	r.propMu.Lock()
	seqBusy := r.cfg.Propagation.SequentialPNStampValidation && len(r.validatingFrom) > 0
	syncBusy := maxInboundSyncs(r) > 0 && r.propagationResourcesTransferring() >= maxInboundSyncs(r)
	r.propMu.Unlock()

	if !bypassSeq && seqBusy {
		return int(PeerErrorThrottled)
	}
	if !bypassSeq && syncBusy {
		return int(PeerErrorThrottled)
	}

	if r.throttled(remoteKey) {
		return int(PeerErrorThrottled)
	}

	if r.cfg.Propagation.FromStaticOnly && !r.isStaticPeer(remotePropHash) {
		return int(PeerErrorNoAccess)
	}

	var req []any
	if err := msgpack.Unmarshal(data, &req); err != nil || len(req) < 2 {
		return int(PeerErrorInvalidData)
	}
	peeringKey, ok := req[0].([]byte)
	if !ok {
		return int(PeerErrorInvalidData)
	}
	transientIDs, ok := req[1].([]any)
	if !ok {
		return int(PeerErrorInvalidData)
	}

	peeringID := append(append([]byte(nil), r.identity.Hash()...), remoteIdentity.Hash()...)
	if !ValidatePeeringKey(peeringID, peeringKey, r.cfg.Propagation.PeeringCost) {
		return int(PeerErrorInvalidKey)
	}

	r.propMu.Lock()
	r.validatedPeerLinks[linkKey] = true
	r.propMu.Unlock()

	wanted := make([][]byte, 0)
	for _, tidAny := range transientIDs {
		tid, ok := tidAny.([]byte)
		if !ok || len(tid) == 0 {
			continue
		}
		if _, ok := r.store.Get(tid); !ok {
			wanted = append(wanted, tid)
		}
	}

	if len(wanted) == 0 {
		return false
	}
	if len(wanted) == len(transientIDs) {
		// Upstream only records an accounting entry for partial
		// accepts, so a full accept returns true without one.
		return true
	}
	r.propMu.Lock()
	r.acceptedOfferLinks[linkKey] = OfferAccepted
	r.propMu.Unlock()
	return wanted
}

func (r *Router) messageGetRequestHandler(path string, data []byte, requestID, linkID []byte, remoteIdentity *identity.Identity, requestedAt int64) any {
	_ = path
	_ = requestID
	_ = requestedAt

	remoteIdentity = r.requestRemoteIdentity(linkID, remoteIdentity)
	if remoteIdentity == nil {
		return int(PeerErrorNoIdentity)
	}
	if !r.identityAllowed(remoteIdentity) {
		return int(PeerErrorNoAccess)
	}

	deliveryHash, err := deliveryDestinationHash(remoteIdentity)
	if err != nil {
		return int(PeerErrorInvalidData)
	}

	var req []any
	if err := msgpack.Unmarshal(data, &req); err != nil || len(req) < 2 {
		return int(PeerErrorInvalidData)
	}

	// Upstream lists available messages only when both fields are
	// None. Explicit empty lists are a get request with no wants.
	if req[0] == nil && req[1] == nil {
		return r.store.ListForDestination(deliveryHash)
	}

	wants := decodeByteList(req[0])
	haves := decodeByteList(req[1])
	var transferLimitKB float64
	limitSet := false
	if len(req) >= 3 {
		// Upstream applies the client limit whenever the field parses as
		// a float, including a literal zero. Absent or non-numeric means
		// unlimited.
		if f, ok := numericField(req[2]); ok {
			transferLimitKB = f
			limitSet = true
		}
	}

	for _, tid := range haves {
		ent, ok := r.store.Get(tid)
		if !ok {
			continue
		}
		if !bytes.Equal(ent.DestinationHash, deliveryHash) {
			continue
		}
		_ = r.store.Remove(tid)
	}

	if len(wants) == 0 {
		return []any{}
	}

	limitBytes := int64(transferLimitKB * 1000)
	const perMessageOverhead = 16
	cumulative := int64(24)
	response := make([][]byte, 0)
	for _, tid := range wants {
		ent, ok := r.store.Get(tid)
		if !ok || !bytes.Equal(ent.DestinationHash, deliveryHash) {
			continue
		}
		lxm, err := r.store.ReadLXMData(tid)
		if err != nil {
			continue
		}
		// Upstream measures the stored file size, which includes the
		// trailing stamp, before stripping it for the response.
		next := cumulative + int64(len(lxm)+StampSize+perMessageOverhead)
		if limitSet && next > limitBytes {
			continue
		}
		response = append(response, lxm)
		cumulative = next
	}
	r.clientPropagationServed += int64(len(response))
	return response
}

func (r *Router) lxmfPropagation(lxmData []byte, stampValue int64, stampData []byte, fromPeer *Peer, isPaper bool) bool {
	if len(lxmData) < Overhead {
		return false
	}
	sum := sha256Sum(lxmData)
	tidKey := peerKey(sum[:])

	// Upstream ignores data already in the store or the processed
	// transient id cache.
	if r.store != nil {
		if _, ok := r.store.Get(sum[:]); ok {
			return false
		}
	}
	r.processedMu.Lock()
	if _, seen := r.locallyProcessed[tidKey]; seen {
		r.processedMu.Unlock()
		return false
	}
	r.locallyProcessed[tidKey] = float64(time.Now().Unix())
	r.processedMu.Unlock()

	destHash := lxmData[:DestinationLength]
	localDelivery := false

	r.mu.RLock()
	deliveryDest := r.deliveryDest
	r.mu.RUnlock()

	if deliveryDest != nil {
		localHash := deliveryDest.GetHash()
		if bytes.Equal(destHash, localHash) {
			decrypted, err := deliveryDest.Decrypt(lxmData[DestinationLength:])
			if err == nil && len(decrypted) > 0 {
				payload := append(append([]byte(nil), destHash...), decrypted...)
				r.handleDeliveryPayload(payload, MethodPropagated, isPaper, false)
				r.deliveredMu.Lock()
				r.locallyDelivered[tidKey] = float64(time.Now().Unix())
				r.deliveredMu.Unlock()
				localDelivery = true
			}
		}
	}

	if localDelivery {
		return true
	}

	if !r.propagationEnabled || r.store == nil {
		return false
	}
	// Upstream concatenates lxmf_data and stamp_data unconditionally, so
	// ingest paths without a stamp (paper messages) fail to store.
	if len(stampData) == 0 {
		return false
	}

	entry, err := r.store.Add(lxmData, stampData, stampValue)
	if err != nil {
		Warning("propagation store add failed", "error", err)
		return false
	}
	r.enqueuePeerDistribution(sum[:], fromPeer)
	Verbose("stored propagated message", "transient", tidKey, "stamp", stampValue, "path", entry.FilePath)
	return true
}

func (r *Router) identityAllowed(id *identity.Identity) bool {
	if !r.cfg.Propagation.AuthRequired {
		return true
	}
	_, ok := r.allowedList[peerKey(id.Hash())]
	return ok
}

func (r *Router) throttled(peerKey string) bool {
	r.propMu.Lock()
	defer r.propMu.Unlock()
	until, ok := r.throttledPeers[peerKey]
	if !ok {
		return false
	}
	if float64(time.Now().Unix()) >= until {
		delete(r.throttledPeers, peerKey)
		return false
	}
	return true
}

func (r *Router) throttlePeer(destHash []byte) {
	if len(destHash) != DestinationLength {
		return
	}
	key := peerKey(destHash)
	r.propMu.Lock()
	r.throttledPeers[key] = float64(time.Now().Unix()) + r.cfg.Propagation.StampThrottleSeconds()
	r.propMu.Unlock()
}

func (r *Router) propagationResourcesTransferring() int {
	count := 0
	for _, state := range r.acceptedOfferLinks {
		if state > OfferAccepted {
			count++
		}
	}
	return count
}

func maxInboundSyncs(r *Router) int {
	// Upstream default is MAX_INBOUND_SYNCS = 3.
	if r.cfg.Propagation.MaxInboundSyncs > 0 {
		return r.cfg.Propagation.MaxInboundSyncs
	}
	return maxInboundSyncsDefault
}

func (r *Router) enqueuePeerDistribution(transientID []byte, fromPeer *Peer) {
	r.propMu.Lock()
	r.peerDistributionQ = append(r.peerDistributionQ, peerDistEntry{
		transientID: append([]byte(nil), transientID...),
		fromPeer:    fromPeer,
	})
	r.propMu.Unlock()
}

func (r *Router) flushPeerDistribution() {
	r.propMu.Lock()
	queue := r.peerDistributionQ
	r.peerDistributionQ = nil
	r.propMu.Unlock()

	if len(queue) == 0 || r.store == nil {
		return
	}

	r.peersMu.RLock()
	defer r.peersMu.RUnlock()
	for _, item := range queue {
		for key, peer := range r.peers {
			if item.fromPeer != nil && peerKey(item.fromPeer.DestinationHash) == key {
				continue
			}
			r.store.addUnhandledPeer(item.transientID, peer.DestinationHash)
			peer.queueUnhandled(item.transientID)
		}
	}
}

func deliveryDestinationHash(id *identity.Identity) ([]byte, error) {
	dest, err := destination.New(id, destination.Out, destination.Single, AppName, nil, "delivery")
	if err != nil {
		return nil, err
	}
	return dest.GetHash(), nil
}

// numericField mirrors Python float() coercion for request fields, accepting
// ints, floats, and numeric strings.
func numericField(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		iv, ok := asInt64(x)
		return float64(iv), ok
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	case []byte:
		f, err := strconv.ParseFloat(strings.TrimSpace(string(x)), 64)
		return f, err == nil
	}
	return 0, false
}

func decodeByteList(v any) [][]byte {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(list))
	for _, item := range list {
		if b, ok := item.([]byte); ok {
			out = append(out, b)
		}
	}
	return out
}

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// IngestLXMURI decodes an lxm:// paper URI and feeds the contained message
// through the propagation ingest path, matching upstream ingest_lxm_uri.
// Stamp enforcement is skipped for the local delivery leg.
func (r *Router) IngestLXMURI(uri string) bool {
	// Upstream accepts the scheme case-insensitively and strips
	// slashes that appear when a URI wraps across QR lines.
	u := strings.ToLower(uri)
	if !strings.HasPrefix(u, uriSchemaPrefix) {
		return false
	}
	encoded := strings.ReplaceAll(uri[len(uriSchemaPrefix):], "/", "")
	data, err := DecodePaperURI(uriSchemaPrefix + encoded)
	if err != nil || len(data) == 0 {
		return false
	}
	return r.lxmfPropagation(data, 0, nil, nil, true)
}
