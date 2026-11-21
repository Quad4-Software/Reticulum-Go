// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"time"
)

// compileStats returns propagation node statistics compatible with upstream lxmd msgpack.
func (r *Router) compileStats() map[string]any {
	if !r.propagationEnabled {
		return nil
	}
	peerStats := map[any]any{}
	r.peersMu.RLock()
	for _, peer := range r.peers {
		peerType := "discovered"
		if r.isStaticPeer(peer.DestinationHash) {
			peerType = "static"
		}
		// Upstream keys peer stats by the raw destination hash bytes.
		peerStats[peer.DestinationHash] = map[string]any{
			"type":                   peerType,
			"state":                  peer.State,
			"alive":                  peer.Alive,
			"name":                   peer.Name(),
			"last_heard":             int(peer.LastHeard),
			"next_sync_attempt":      peer.NextSyncAttempt,
			"last_sync_attempt":      peer.LastSyncAttempt,
			"sync_backoff":           peer.SyncBackoff,
			"peering_timebase":       peer.PeeringTimebase,
			"ler":                    int(peer.LinkEstablishmentRate),
			"str":                    int(peer.SyncTransferRate),
			"transfer_limit":         peer.PropagationTransferLimit,
			"sync_limit":             peer.PropagationSyncLimit,
			"target_stamp_cost":      peer.PropagationStampCost,
			"stamp_cost_flexibility": peer.PropagationStampCostFlex,
			"peering_cost":           peer.PeeringCost,
			"peering_key":            peer.PeeringKeyValue(),
			"network_distance":       r.transport.HopsTo(peer.DestinationHash),
			"rx_bytes":               peer.RXBytes,
			"tx_bytes":               peer.TXBytes,
			"acceptance_rate":        peer.AcceptanceRate(),
			"messages": map[string]any{
				"offered":   peer.Offered,
				"outgoing":  peer.Outgoing,
				"incoming":  peer.Incoming,
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
	return r.compileStats()
}

func peerKey(hash []byte) string {
	return hex.EncodeToString(hash)
}
