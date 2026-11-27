// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"time"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
)

type binKey string

func (k binKey) EncodeMsgpack(enc *msgpack.Encoder) error {
	return enc.EncodeBytes([]byte(k))
}

// compileStats returns propagation node statistics compatible with upstream lxmd msgpack.
func (r *Router) compileStats() map[string]any {
	if !r.propagationEnabled {
		return nil
	}
	peerStats := map[binKey]any{}
	r.peersMu.RLock()
	for _, peer := range r.peers {
		peerType := "discovered"
		if r.isStaticPeer(peer.DestinationHash) {
			peerType = "static"
		}
		peer.mu.RLock()
		state := peer.State
		alive := peer.Alive
		lastHeard := peer.LastHeard
		nextSyncAttempt := peer.NextSyncAttempt
		lastSyncAttempt := peer.LastSyncAttempt
		syncBackoff := peer.SyncBackoff
		peeringTimebase := peer.PeeringTimebase
		ler := peer.LinkEstablishmentRate
		str := peer.SyncTransferRate
		transferLimit := peer.PropagationTransferLimit
		syncLimit := peer.PropagationSyncLimit
		stampCost := peer.PropagationStampCost
		stampCostFlex := peer.PropagationStampCostFlex
		peeringCost := peer.PeeringCost
		rxBytes := peer.RXBytes
		txBytes := peer.TXBytes
		offered := peer.Offered
		outgoing := peer.Outgoing
		incoming := peer.Incoming
		peer.mu.RUnlock()
		// Upstream keys peer stats by the raw destination hash bytes.
		peerStats[binKey(peer.DestinationHash)] = map[string]any{
			"type":                   peerType,
			"state":                  state,
			"alive":                  alive,
			"name":                   peer.Name(),
			"last_heard":             int(lastHeard),
			"next_sync_attempt":      nextSyncAttempt,
			"last_sync_attempt":      lastSyncAttempt,
			"sync_backoff":           syncBackoff,
			"peering_timebase":       peeringTimebase,
			"ler":                    int(ler),
			"str":                    int(str),
			"transfer_limit":         transferLimit,
			"sync_limit":             syncLimit,
			"target_stamp_cost":      stampCost,
			"stamp_cost_flexibility": stampCostFlex,
			"peering_cost":           peeringCost,
			"peering_key":            peer.PeeringKeyValue(),
			"network_distance":       r.transport.HopsTo(peer.DestinationHash),
			"rx_bytes":               rxBytes,
			"tx_bytes":               txBytes,
			"acceptance_rate":        peer.AcceptanceRate(),
			"messages": map[string]any{
				"offered":   offered,
				"outgoing":  outgoing,
				"incoming":  incoming,
				"unhandled": peer.UnhandledCount(r),
			},
		}
	}
	staticCount := len(r.staticPeers)
	totalPeers := len(r.peers)
	r.peersMu.RUnlock()

	var storeCount int
	var storeBytes int64
	var storeLimit any
	if r.store != nil {
		storeCount = r.store.Count()
		storeBytes = r.store.TotalBytes()
		if r.cfg.Propagation.MessageStorageLimitMB > 0 {
			storeLimit = int64(r.cfg.Propagation.MessageStorageLimitMB * 1000 * 1000)
		}
	}

	uptime := 0.0
	if !r.propagationStart.IsZero() {
		uptime = time.Since(r.propagationStart).Seconds()
	}

	return map[string]any{
		"identity_hash":          r.identity.Hash(),
		"destination_hash":       r.propagationDest.GetHash(),
		"uptime":                 uptime,
		"delivery_limit":         r.cfg.LXMF.DeliveryTransferMaxAcceptedSize,
		"propagation_limit":      r.cfg.Propagation.PropagationTransferMaxAcceptedKB,
		"sync_limit":             r.cfg.Propagation.PropagationSyncMaxAcceptedKB,
		"target_stamp_cost":      r.cfg.Propagation.PropagationStampCostTarget,
		"stamp_cost_flexibility": r.cfg.Propagation.PropagationStampCostFlexibility,
		"peering_cost":           r.cfg.Propagation.PeeringCost,
		"max_peering_cost":       r.cfg.Propagation.RemotePeeringCostMax,
		"autopeer_maxdepth":      r.cfg.Propagation.AutopeerMaxDepth,
		"from_static_only":       r.cfg.Propagation.FromStaticOnly,
		"messagestore": map[string]any{
			"count": storeCount,
			"bytes": storeBytes,
			"limit": storeLimit,
		},
		"clients": map[string]any{
			"client_propagation_messages_received": r.clientPropagationReceived,
			"client_propagation_messages_served":   r.clientPropagationServed,
		},
		"unpeered_propagation_incoming": r.unpeeredPropagationIncoming,
		"unpeered_propagation_rx_bytes": r.unpeeredPropagationRxBytes,
		"static_peers":                  staticCount,
		"discovered_peers":              totalPeers - staticCount,
		"total_peers":                   totalPeers,
		"max_peers":                     r.cfg.Propagation.MaxPeers,
		"peers":                         peerStats,
	}
}

// PropagationStats returns propagation node statistics for local inspection.
func (r *Router) PropagationStats() map[string]any {
	stats := r.compileStats()
	if stats == nil {
		return nil
	}
	local := make(map[string]any, len(stats))
	for k, v := range stats {
		local[k] = v
	}
	peers := make(map[string]any)
	if wirePeers, ok := stats["peers"].(map[binKey]any); ok {
		for key, peer := range wirePeers {
			peers[hex.EncodeToString([]byte(key))] = peer
		}
	}
	local["peers"] = peers
	return local
}

func peerKey(hash []byte) string {
	return hex.EncodeToString(hash)
}
