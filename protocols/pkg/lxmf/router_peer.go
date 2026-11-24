// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
)

const (
	peerMaxUnreachable     = 14 * 24 * 60 * 60
	peerSyncBackoffStep    = 12 * 60
	peerPathRequestGrace   = 7500 * time.Millisecond
	peerLinkAttemptTimeout = 45 * time.Second
)

// Peer is a persisted propagation peer (msgpack on disk).
type Peer struct {
	router *Router

	DestinationHash []byte
	Identity        *identity.Identity
	Destination     *destination.Destination

	Alive        bool
	LastHeard    float64
	Metadata     map[byte]any
	SyncStrategy byte

	PeeringKey   []byte
	PeeringValue int
	PeeringCost  int

	NextSyncAttempt float64
	LastSyncAttempt float64
	SyncBackoff     float64
	PeeringTimebase int64

	LinkEstablishmentRate float64
	SyncTransferRate      float64

	PropagationTransferLimit float64
	PropagationSyncLimit     float64
	PropagationStampCost     int
	PropagationStampCostFlex int

	State byte
	Link  *link.Link

	LastOffer []string

	Offered  int64
	Outgoing int64
	Incoming int64
	RXBytes  int64
	TXBytes  int64

	peeringKeyMu sync.Mutex
}

// UnhandledCount returns messages queued for this peer.
func (p *Peer) UnhandledCount(r *Router) int {
	if r == nil || r.store == nil {
		return 0
	}
	count := 0
	peerHex := peerKey(p.DestinationHash)
	for _, ent := range r.store.entriesSnapshot() {
		if slices.Contains(ent.UnhandledPeers, peerHex) {
			count++
		}
	}
	return count
}

// AcceptanceRate returns outgoing/offered ratio.
func (p *Peer) AcceptanceRate() float64 {
	if p.Offered == 0 {
		return 0
	}
	return float64(p.Outgoing) / float64(p.Offered)
}

// Name returns PN metadata name when present.
func (p *Peer) Name() string {
	if p.Metadata == nil {
		return ""
	}
	v, ok := p.Metadata[PNMetaName]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return ""
	}
}

// PeeringKeyValue returns the generated peering key score.
func (p *Peer) PeeringKeyValue() int {
	return p.PeeringValue
}

func (p *Peer) peeringKeyReady() bool {
	if p.PeeringCost <= 0 {
		return false
	}
	return p.PeeringKey != nil && p.PeeringValue >= p.PeeringCost
}

func (p *Peer) generatePeeringKey() bool {
	if p.PeeringCost <= 0 || p.router == nil || p.router.identity == nil {
		return false
	}
	p.peeringKeyMu.Lock()
	defer p.peeringKeyMu.Unlock()
	if p.PeeringKey != nil && p.PeeringValue >= p.PeeringCost {
		return true
	}
	if p.Identity == nil {
		id, err := identity.Recall(p.DestinationHash)
		if err != nil || id == nil {
			return false
		}
		p.Identity = id
	}
	material := append(append([]byte(nil), p.Identity.Hash()...), p.router.identity.Hash()...)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	stamp, value, err := GenerateStamp(ctx, material, p.PeeringCost, WorkblockExpandRoundsPeering)
	if err != nil {
		return false
	}
	p.PeeringKey = stamp
	p.PeeringValue = value
	return value >= p.PeeringCost
}

func (p *Peer) queueUnhandled(transientID []byte) {
	if p.router == nil || p.router.store == nil {
		return
	}
	p.router.store.addUnhandledPeer(transientID, p.DestinationHash)
}

func (p *Peer) queueHandled(transientID []byte) {
	if p.router == nil || p.router.store == nil {
		return
	}
	p.router.store.markHandled(transientID, p.DestinationHash)
}

func (p *Peer) sync() {
	if p.router == nil {
		return
	}
	// Upstream establishes a new link from IDLE and re-offers on the
	// existing link from LINK_READY. Other states are in flight.
	if p.State != PeerStateIdle && p.State != PeerStateLinkReady {
		return
	}
	p.LastSyncAttempt = float64(time.Now().Unix())

	if time.Now().Unix() < int64(p.NextSyncAttempt) {
		// LXMF: a peer postponed by backoff that we have not heard from
		// since the last sync attempt is marked not alive.
		if p.LastSyncAttempt > p.LastHeard {
			p.Alive = false
		}
		return
	}
	// Upstream requires all announced stamp and peering costs to be
	// known before syncing. Unset costs are stored as -1.
	if p.PropagationStampCost < 0 || p.PropagationStampCostFlex < 0 || p.PeeringCost < 0 {
		return
	}
	if !p.peeringKeyReady() {
		go p.generatePeeringKey()
		return
	}

	tr := p.router.transport
	if !tr.HasPath(p.DestinationHash) {
		// Upstream LXMPeer.sync calls router.request_path, which
		// debounces repeats inside the request window.
		p.router.requestPath(p.DestinationHash, true)
		time.Sleep(peerPathRequestGrace)
	}
	if !tr.HasPath(p.DestinationHash) {
		// LXMF 1.3.0: an unanswered path request backs off the sync
		// schedule and marks the peer not alive.
		p.SyncBackoff += peerSyncBackoffStep
		p.NextSyncAttempt = float64(time.Now().Unix()) + p.SyncBackoff
		p.Alive = false
		Debug("path request was not answered, retrying sync with peer later", "peer", peerKey(p.DestinationHash))
		return
	}

	if p.Identity == nil {
		id, err := identity.Recall(p.DestinationHash)
		if err != nil || id == nil {
			return
		}
		p.Identity = id
	}
	if p.Destination == nil {
		dest, err := destination.FromHash(p.DestinationHash, p.Identity, destination.Single, tr)
		if err != nil {
			return
		}
		p.Destination = dest
	}
	if p.UnhandledCount(p.router) == 0 {
		// Upstream completes the sync without establishing a link when
		// no unhandled messages exist for the peer.
		return
	}

	if p.State == PeerStateLinkReady {
		// Upstream reuses the active link for a new offer round.
		if p.Link != nil && p.Link.IsActive() {
			p.sendOffer(p.Link)
			return
		}
		p.Link = nil
		p.State = PeerStateIdle
	}

	p.SyncBackoff += peerSyncBackoffStep
	p.NextSyncAttempt = float64(time.Now().Unix()) + p.SyncBackoff

	lnk := link.NewLink(p.Destination, tr, nil, nil, func(closed *link.Link) {
		if p.Link == closed {
			p.Link = nil
			p.State = PeerStateIdle
		}
	})
	if err := lnk.Establish(); err != nil {
		return
	}
	lnk.Start()
	p.Link = lnk
	p.State = PeerStateLinkEstablishing

	deadline := time.Now().Add(peerLinkAttemptTimeout)
	for !lnk.IsActive() && time.Now().Before(deadline) {
		time.Sleep(pathPollInterval)
	}
	if !lnk.IsActive() {
		lnk.Teardown()
		p.Link = nil
		p.State = PeerStateIdle
		return
	}

	// Upstream link_established identifies on the link and records the
	// establishment rate before offering.
	_ = lnk.Identify(p.router.identity)
	if rate := lnk.GetEstablishmentRate(); rate > 0 {
		p.LinkEstablishmentRate = rate
	}
	p.State = PeerStateLinkReady
	p.Alive = true
	p.LastHeard = float64(time.Now().Unix())
	p.NextSyncAttempt = 0
	p.SyncBackoff = 0
	p.sendOffer(lnk)
}

func (p *Peer) sendOffer(lnk *link.Link) {
	if p.router == nil || p.router.store == nil {
		return
	}
	minCost := max(0, p.PropagationStampCost-p.PropagationStampCostFlex)
	pk := peerKey(p.DestinationHash)

	type offerEntry struct {
		key    string
		tid    []byte
		weight float64
		size   int64
	}
	entries := make([]offerEntry, 0)
	for key, ent := range p.router.store.entriesSnapshot() {
		if !slices.Contains(ent.UnhandledPeers, pk) {
			continue
		}
		tid, err := hex.DecodeString(key)
		if err != nil {
			continue
		}
		if ent.StampValue < int64(minCost) {
			// Upstream drops unhandled entries whose stamp value is
			// lower than the peer requirement so they are not offered
			// again on the next sync.
			p.router.store.removeUnhandledPeer(tid, p.DestinationHash)
			continue
		}
		entries = append(entries, offerEntry{key: key, tid: tid, weight: p.router.store.Weight(tid), size: ent.Size})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].weight < entries[j].weight })

	const perMessageOverhead = 16
	cumulative := int64(24)
	ids := make([][]byte, 0, len(entries))
	p.LastOffer = nil
	for _, e := range entries {
		transferSize := e.size + perMessageOverhead
		if p.PropagationTransferLimit >= 0 && float64(transferSize) > p.PropagationTransferLimit*1000 {
			// Exceeds the peer per message transfer limit, so it is
			// marked handled and never offered to this peer again.
			p.router.store.markHandled(e.tid, p.DestinationHash)
			continue
		}
		if p.PropagationSyncLimit >= 0 && float64(cumulative+transferSize) >= p.PropagationSyncLimit*1000 {
			continue
		}
		cumulative += transferSize
		ids = append(ids, e.tid)
		p.LastOffer = append(p.LastOffer, e.key)
	}
	if len(ids) == 0 {
		// Upstream completes the sync without sending an offer and
		// leaves the link active in LINK_READY.
		p.State = PeerStateLinkReady
		return
	}

	offer := []any{p.PeeringKey, ids}
	receipt, err := lnk.Request(PathOffer, offer, 0)
	if err != nil {
		lnk.Teardown()
		p.State = PeerStateIdle
		return
	}
	p.State = PeerStateRequestSent
	receipt.SetResponseCallback(func(r *link.RequestReceipt) {
		p.handleOfferResponse(r)
	})
	receipt.SetFailedCallback(func(r *link.RequestReceipt) {
		_ = r
		if p.Link != nil {
			p.Link.Teardown()
		}
		p.State = PeerStateIdle
	})
}

func (p *Peer) handleOfferResponse(receipt *link.RequestReceipt) {
	p.State = PeerStateResponseReceived
	resp := receipt.GetResponseValue()

	var wantedKeys []string
	switch v := resp.(type) {
	case bool:
		if !v {
			// Peer already has all advertised messages.
			for _, key := range p.LastOffer {
				tid, _ := hex.DecodeString(key)
				p.router.store.markHandled(tid, p.DestinationHash)
			}
			p.Offered += int64(len(p.LastOffer))
			if p.Link != nil {
				p.Link.Teardown()
				p.Link = nil
			}
			p.State = PeerStateIdle
			return
		}
		// Peer wants all advertised messages.
		wantedKeys = p.LastOffer
	case []any:
		// Peer wants some advertised messages. Messages it did not
		// want are already known to it and are marked handled.
		wanted := make(map[string]struct{}, len(v))
		for _, item := range v {
			if b, ok := item.([]byte); ok {
				wanted[peerKey(b)] = struct{}{}
			}
		}
		for _, key := range p.LastOffer {
			if _, ok := wanted[key]; !ok {
				tid, _ := hex.DecodeString(key)
				p.router.store.markHandled(tid, p.DestinationHash)
				continue
			}
			wantedKeys = append(wantedKeys, key)
		}
	default:
		if code, ok := responseErrorCode(resp); ok {
			switch code {
			case PeerErrorNoIdentity:
				// The remote peer did not receive our identity, so
				// re-identify on the link and retry the offer.
				if p.Link != nil {
					Verbose("remote peer indicated missing identification, retrying")
					_ = p.Link.Identify(p.router.identity)
					p.State = PeerStateLinkReady
					p.sendOffer(p.Link)
				}
				return
			case PeerErrorThrottled:
				p.NextSyncAttempt = float64(time.Now().Unix()) + pnStampThrottleSeconds
				return
			case PeerErrorNoAccess:
				p.router.unpeer(p.DestinationHash, 0)
				return
			}
		}
		// Unknown, empty, or invalid response: abort the sync.
		if p.Link != nil {
			p.Link.Teardown()
			p.Link = nil
		}
		p.State = PeerStateIdle
		return
	}

	payloads := make([][]byte, 0, len(wantedKeys))
	for _, key := range wantedKeys {
		if _, err := hex.DecodeString(key); err != nil {
			continue
		}
		data, err := os.ReadFile(p.router.store.entryPath(key))
		if err != nil {
			continue
		}
		payloads = append(payloads, data)
	}
	if len(payloads) == 0 {
		// Peer did not request any of the available messages.
		p.Offered += int64(len(p.LastOffer))
		if p.Link != nil {
			p.Link.Teardown()
			p.Link = nil
		}
		p.State = PeerStateIdle
		return
	}

	packed, err := msgpack.Marshal([]any{float64(time.Now().Unix()), payloads})
	if err != nil || p.Link == nil {
		return
	}
	res, err := resource.New(packed, true)
	if err != nil {
		return
	}
	p.State = PeerStateResourceTransferring
	transferStart := time.Now()
	if err := p.Link.SendResource(res); err != nil {
		p.Link.Teardown()
		p.Link = nil
		p.State = PeerStateIdle
		return
	}

	for _, key := range wantedKeys {
		tid, _ := hex.DecodeString(key)
		p.router.store.markHandled(tid, p.DestinationHash)
	}
	if elapsed := time.Since(transferStart).Seconds(); elapsed > 0 {
		p.SyncTransferRate = float64(len(packed)*8) / elapsed
	}
	p.Alive = true
	p.LastHeard = float64(time.Now().Unix())
	p.Offered += int64(len(p.LastOffer))
	p.Outgoing += int64(len(wantedKeys))
	p.TXBytes += int64(len(packed))

	p.Link.Teardown()
	p.Link = nil
	p.State = PeerStateIdle

	if p.SyncStrategy == PeerStrategyPersistent && p.UnhandledCount(p.router) > 0 {
		go p.sync()
	}
}

// responseErrorCode extracts a propagation error code from a request
// response. Upstream sends bare msgpack ints, while older Go peers sent
// single byte payloads, so both forms are accepted.
func responseErrorCode(v any) (byte, bool) {
	if b, ok := v.([]byte); ok && len(b) == 1 {
		return b[0], true
	}
	if n, ok := asInt64(v); ok && n >= 0 && n <= 0xff {
		return byte(n), true
	}
	return 0, false
}

func peerFromBytes(r *Router, data []byte) (*Peer, error) {
	var dict map[string]any
	if err := msgpack.Unmarshal(data, &dict); err != nil {
		return nil, err
	}
	dest, ok := dict["destination_hash"].([]byte)
	if !ok || len(dest) != DestinationLength {
		return nil, fmt.Errorf("lxmf: invalid peer destination hash")
	}
	peer := newPeer(r, dest)
	// Unset optional values decode as -1, matching upstream None.
	peer.PropagationStampCost = -1
	peer.PropagationStampCostFlex = -1
	peer.PeeringCost = -1
	peer.PropagationTransferLimit = -1
	peer.PropagationSyncLimit = -1
	if v, ok := asInt64(dict["peering_timebase"]); ok {
		peer.PeeringTimebase = v
	}
	if v, ok := dict["alive"].(bool); ok {
		peer.Alive = v
	}
	if v := dict["last_heard"]; v != nil {
		peer.LastHeard = asFloat(v)
	}
	// Upstream stores the peering key as a [key, value] list. Older Go
	// peers stored the key bytes alone, so both forms are accepted.
	switch v := dict["peering_key"].(type) {
	case []byte:
		peer.PeeringKey = v
	case []any:
		if len(v) == 2 {
			if key, ok := v[0].([]byte); ok {
				peer.PeeringKey = key
			}
			if val, ok := asInt64(v[1]); ok {
				peer.PeeringValue = int(val)
			}
		}
	}
	if v, ok := dict["metadata"].(map[any]any); ok {
		peer.Metadata = mapAnyToByteKey(v)
	}
	if v, ok := dict["sync_strategy"].(int64); ok && v >= 0 && v <= 0xFF {
		peer.SyncStrategy = byte(v)
	}
	if v, ok := asInt64(dict["propagation_stamp_cost"]); ok {
		peer.PropagationStampCost = int(v)
	}
	if v, ok := asInt64(dict["propagation_stamp_cost_flexibility"]); ok {
		peer.PropagationStampCostFlex = int(v)
	}
	if v, ok := asInt64(dict["peering_cost"]); ok {
		peer.PeeringCost = int(v)
	}
	// Upstream coerces the limit fields with float()/int() and treats
	// unparseable values as unset.
	if f, ok := numericField(dict["propagation_transfer_limit"]); ok {
		peer.PropagationTransferLimit = f
	}
	if f, ok := numericField(dict["propagation_sync_limit"]); ok {
		peer.PropagationSyncLimit = f
	}
	// Upstream falls back to the transfer limit when the sync limit is
	// missing or unparseable in the peer file.
	if peer.PropagationSyncLimit < 0 {
		peer.PropagationSyncLimit = peer.PropagationTransferLimit
	}
	if v := dict["link_establishment_rate"]; v != nil {
		peer.LinkEstablishmentRate = asFloat(v)
	}
	if v := dict["sync_transfer_rate"]; v != nil {
		peer.SyncTransferRate = asFloat(v)
	}
	if v := dict["last_sync_attempt"]; v != nil {
		peer.LastSyncAttempt = asFloat(v)
	}
	if v, ok := asInt64(dict["offered"]); ok {
		peer.Offered = v
	}
	if v, ok := asInt64(dict["outgoing"]); ok {
		peer.Outgoing = v
	}
	if v, ok := asInt64(dict["incoming"]); ok {
		peer.Incoming = v
	}
	if v, ok := asInt64(dict["rx_bytes"]); ok {
		peer.RXBytes = v
	}
	if v, ok := asInt64(dict["tx_bytes"]); ok {
		peer.TXBytes = v
	}
	if r.store != nil {
		for _, tid := range decodeByteList(dict["handled_ids"]) {
			r.store.markHandled(tid, peer.DestinationHash)
		}
		for _, tid := range decodeByteList(dict["unhandled_ids"]) {
			r.store.addUnhandledPeer(tid, peer.DestinationHash)
		}
	}
	return peer, nil
}

func (p *Peer) toBytes() ([]byte, error) {
	dict := map[string]any{
		"destination_hash":                   p.DestinationHash,
		"peering_timebase":                   p.PeeringTimebase,
		"alive":                              p.Alive,
		"last_heard":                         p.LastHeard,
		"sync_strategy":                      p.SyncStrategy,
		"peering_key":                        nil,
		"metadata":                           p.Metadata,
		"link_establishment_rate":            p.LinkEstablishmentRate,
		"sync_transfer_rate":                 p.SyncTransferRate,
		"propagation_transfer_limit":         nil,
		"propagation_sync_limit":             nil,
		"propagation_stamp_cost":             nil,
		"propagation_stamp_cost_flexibility": nil,
		"peering_cost":                       nil,
		"last_sync_attempt":                  p.LastSyncAttempt,
		"offered":                            p.Offered,
		"outgoing":                           p.Outgoing,
		"incoming":                           p.Incoming,
		"rx_bytes":                           p.RXBytes,
		"tx_bytes":                           p.TXBytes,
		"handled_ids":                        []any{},
		"unhandled_ids":                      []any{},
	}
	// Upstream stores the peering key as a [key, value] list and unset
	// limits and costs as None, so nil is written for unset values.
	if p.PeeringKey != nil {
		dict["peering_key"] = []any{p.PeeringKey, p.PeeringValue}
	}
	if p.PropagationTransferLimit >= 0 {
		dict["propagation_transfer_limit"] = p.PropagationTransferLimit
	}
	if p.PropagationSyncLimit >= 0 {
		dict["propagation_sync_limit"] = p.PropagationSyncLimit
	}
	if p.PropagationStampCost >= 0 {
		dict["propagation_stamp_cost"] = p.PropagationStampCost
	}
	if p.PropagationStampCostFlex >= 0 {
		dict["propagation_stamp_cost_flexibility"] = p.PropagationStampCostFlex
	}
	if p.PeeringCost >= 0 {
		dict["peering_cost"] = p.PeeringCost
	}
	if p.router != nil && p.router.store != nil {
		pk := peerKey(p.DestinationHash)
		handled := make([]any, 0)
		unhandled := make([]any, 0)
		for key, ent := range p.router.store.entriesSnapshot() {
			tid, err := hex.DecodeString(key)
			if err != nil {
				continue
			}
			if slices.Contains(ent.HandledPeers, pk) {
				handled = append(handled, tid)
			}
			if slices.Contains(ent.UnhandledPeers, pk) {
				unhandled = append(unhandled, tid)
			}
		}
		dict["handled_ids"] = handled
		dict["unhandled_ids"] = unhandled
	}
	return msgpack.Marshal(dict)
}

func newPeer(r *Router, destHash []byte) *Peer {
	return &Peer{
		router:          r,
		DestinationHash: append([]byte(nil), destHash...),
		SyncStrategy:    PeerStrategyPersistent,
	}
}

func (r *Router) peer(destHash []byte, timebase int64, transferLimit, syncLimit float64, stampCost, stampFlex, peeringCost int, metadata map[byte]any) {
	if r.isOwnPropagationHash(destHash) {
		return
	}
	if peeringCost > r.cfg.Propagation.RemotePeeringCostMax {
		r.peersMu.RLock()
		_, isPeer := r.peers[peerKey(destHash)]
		r.peersMu.RUnlock()
		if isPeer {
			r.unpeer(destHash, timebase)
		}
		return
	}

	// Upstream falls back to the transfer limit when no sync limit is
	// known, so a max size transfer always fits in one sync.
	if syncLimit < 0 {
		syncLimit = transferLimit
	}

	key := peerKey(destHash)
	r.peersMu.Lock()

	if peer, ok := r.peers[key]; ok {
		if timebase > peer.PeeringTimebase {
			peer.Alive = true
			peer.Metadata = metadata
			peer.SyncBackoff = 0
			peer.NextSyncAttempt = 0
			peer.PeeringTimebase = timebase
			peer.LastHeard = float64(time.Now().Unix())
			peer.PropagationStampCost = stampCost
			peer.PropagationStampCostFlex = stampFlex
			peer.PeeringCost = peeringCost
			peer.PropagationTransferLimit = transferLimit
			peer.PropagationSyncLimit = syncLimit
			r.peersMu.Unlock()
			r.seedPeerSync(peer)
			return
		}
		r.peersMu.Unlock()
		return
	}

	if len(r.peers) >= r.cfg.Propagation.MaxPeers {
		r.peersMu.Unlock()
		return
	}

	peer := newPeer(r, destHash)
	peer.Alive = true
	peer.Metadata = metadata
	peer.LastHeard = float64(time.Now().Unix())
	peer.PeeringTimebase = timebase
	peer.PropagationStampCost = stampCost
	peer.PropagationStampCostFlex = stampFlex
	peer.PeeringCost = peeringCost
	peer.PropagationTransferLimit = transferLimit
	peer.PropagationSyncLimit = syncLimit
	r.peers[key] = peer
	r.peersMu.Unlock()
	Info("peered with propagation node", "peer", key)
	r.seedPeerSync(peer)
}

func (r *Router) queuePeerStoreSync(peer *Peer) int {
	if peer == nil || r.store == nil {
		return 0
	}
	queued := 0
	pk := peerKey(peer.DestinationHash)
	for key, ent := range r.store.entriesSnapshot() {
		if slices.Contains(ent.HandledPeers, pk) {
			continue
		}
		tid, err := hex.DecodeString(key)
		if err != nil {
			continue
		}
		r.store.addUnhandledPeer(tid, peer.DestinationHash)
		queued++
	}
	return queued
}

func (r *Router) seedPeerSync(peer *Peer) {
	if peer == nil {
		return
	}
	r.queuePeerStoreSync(peer)
	go peer.sync()
}

func (p *Peer) needsSync(r *Router) bool {
	if p.UnhandledCount(r) > 0 {
		return true
	}
	if r == nil || r.store == nil {
		return p.LastSyncAttempt == 0
	}
	pk := peerKey(p.DestinationHash)
	for _, ent := range r.store.entriesSnapshot() {
		if !slices.Contains(ent.HandledPeers, pk) {
			return true
		}
	}
	return false
}

func (r *Router) unpeer(destHash []byte, timestamp int64) {
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	key := peerKey(destHash)
	r.peersMu.Lock()
	defer r.peersMu.Unlock()
	peer, ok := r.peers[key]
	if !ok {
		return
	}
	if int64(timestamp) >= peer.PeeringTimebase {
		delete(r.peers, key)
		_ = os.Remove(filepath.Join(r.peersDir(), key))
		Info("unpeered propagation node", "peer", key)
	}
}

func (r *Router) peerFromStatic(destHash []byte) {
	// Upstream seeds static peers directly with an empty timebase and
	// requests a path so the first announce can update the peering
	// config even when it arrives as a path response.
	r.peersMu.Lock()
	if _, ok := r.peers[peerKey(destHash)]; !ok {
		r.peers[peerKey(destHash)] = newPeer(r, destHash)
	}
	r.peersMu.Unlock()
	if r.transport != nil {
		r.requestPath(destHash, false)
	}
}

// fastestPeerPoolSize matches upstream LXMRouter.FASTEST_N_RANDOM_POOL.
const fastestPeerPoolSize = 2

func (r *Router) syncPeers() {
	r.peersMu.RLock()
	peers := make([]*Peer, 0, len(r.peers))
	for _, p := range r.peers {
		peers = append(peers, p)
	}
	r.peersMu.RUnlock()

	var waiting, unresponsive []*Peer
	for _, peer := range peers {
		if time.Now().Unix() > int64(peer.LastHeard)+peerMaxUnreachable && !r.isStaticPeer(peer.DestinationHash) {
			r.unpeer(peer.DestinationHash, 0)
			continue
		}
		if peer.State != PeerStateIdle || peer.UnhandledCount(r) == 0 {
			continue
		}
		if peer.Alive {
			waiting = append(waiting, peer)
		} else if float64(time.Now().Unix()) > peer.NextSyncAttempt {
			unresponsive = append(unresponsive, peer)
		}
	}

	// Upstream syncs a single peer per round, selected randomly from a
	// pool of the fastest known alive peers plus an equal number of
	// peers with unknown sync speed. Unresponsive peers past their
	// next sync attempt are only tried when no alive peer is waiting.
	var pool []*Peer
	if len(waiting) > 0 {
		sort.Slice(waiting, func(i, j int) bool {
			return waiting[i].SyncTransferRate > waiting[j].SyncTransferRate
		})
		fastest := waiting[:min(len(waiting), fastestPeerPoolSize)]
		pool = append(pool, fastest...)
		unknowns := 0
		for _, p := range waiting {
			if p.SyncTransferRate == 0 && unknowns < len(fastest) {
				pool = append(pool, p)
				unknowns++
			}
		}
	} else {
		pool = unresponsive
	}
	if len(pool) > 0 {
		go pool[rand.IntN(len(pool))].sync() // #nosec G404 -- non-cryptographic peer selection
	}
}

// rotatePeers drops low acceptance rate peers when the peer count
// approaches max_peers, matching upstream LXMRouter.rotate_peers.
func (r *Router) rotatePeers() {
	maxPeers := r.cfg.Propagation.MaxPeers
	if maxPeers <= 0 {
		return
	}
	// Upstream ROTATION_HEADROOM_PCT is 10.
	headroom := max(1, maxPeers*10/100)

	r.peersMu.RLock()
	peerCount := len(r.peers)
	required := peerCount - (maxPeers - headroom)
	if required <= 0 || peerCount-required <= 1 {
		r.peersMu.RUnlock()
		return
	}
	untested := 0
	pool := make([]*Peer, 0, peerCount)
	for _, p := range r.peers {
		pool = append(pool, p)
		if p.LastSyncAttempt == 0 {
			untested++
		}
	}
	r.peersMu.RUnlock()

	if untested >= headroom {
		// Newly added peers have not been tested yet, postpone rotation.
		return
	}

	// Fully synced peers are preferred as the rotation pool basis.
	var fullySynced []*Peer
	for _, p := range pool {
		if p.UnhandledCount(r) == 0 {
			fullySynced = append(fullySynced, p)
		}
	}
	if len(fullySynced) > 0 {
		pool = fullySynced
	}

	var waiting, unresponsive []*Peer
	for _, p := range pool {
		if r.isStaticPeer(p.DestinationHash) || p.State != PeerStateIdle {
			continue
		}
		if p.Alive {
			// Peers are not considered for unpeering until at least one
			// message has been offered to them.
			if p.Offered > 0 {
				waiting = append(waiting, p)
			}
		} else {
			unresponsive = append(unresponsive, p)
		}
	}

	// Upstream prioritise_rotating_unreachable_peers is false, so the
	// drop pool always contains both unresponsive and waiting peers.
	dropPool := append(unresponsive, waiting...)
	if len(dropPool) == 0 {
		return
	}
	sort.Slice(dropPool, func(i, j int) bool {
		return dropPool[i].AcceptanceRate() < dropPool[j].AcceptanceRate()
	})
	drops := min(required, len(dropPool))
	for _, p := range dropPool[:drops] {
		// Upstream ROTATION_AR_MAX is 0.5.
		if p.AcceptanceRate() < 0.5 {
			Debug("dropping low acceptance rate peer", "peer", peerKey(p.DestinationHash), "acceptance_rate", p.AcceptanceRate())
			r.unpeer(p.DestinationHash, 0)
		}
	}
}

func mapAnyToByteKey(m map[any]any) map[byte]any {
	out := make(map[byte]any, len(m))
	for k, v := range m {
		ki, ok := asInt64(k)
		if !ok || ki < 0 || ki > 0xff {
			continue
		}
		out[byte(ki)] = v
	}
	return out
}
