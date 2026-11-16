// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"encoding/hex"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
)

type deliveryAnnounceHandler struct {
	router *Router
}

func (h *deliveryAnnounceHandler) AspectFilter() []string { return []string{AppName + ".delivery"} }

func (h *deliveryAnnounceHandler) ReceivePathResponses() bool { return true }

func (h *deliveryAnnounceHandler) ReceivedAnnounce(destHash []byte, identAny any, appData []byte, hops uint8) error {
	_ = hops
	_ = identAny
	if h.router == nil {
		return nil
	}
	// Upstream records the stamp cost announced by every lxmf.delivery
	// destination into outbound_stamp_costs for later sends.
	cost, ok, err := StampCostFromAppData(appData)
	if err != nil {
		return nil
	}
	h.router.stampCosts.update(destHash, cost, ok)
	return nil
}

type propagationAnnounceHandler struct {
	router *Router
}

func (h *propagationAnnounceHandler) AspectFilter() []string {
	return []string{AppName + ".propagation"}
}

func (h *propagationAnnounceHandler) ReceivePathResponses() bool { return true }

func (h *propagationAnnounceHandler) ReceivedAnnounce(destHash []byte, identAny any, appData []byte, hops uint8) error {
	return h.receivedAnnounce(destHash, identAny, appData, hops, false)
}

// ReceivedAnnouncePathResponse implements announce.PathAwareHandler so
// peering decisions can skip path responses, matching upstream.
func (h *propagationAnnounceHandler) ReceivedAnnouncePathResponse(destHash []byte, identAny any, appData []byte, hops uint8, isPathResponse bool) error {
	return h.receivedAnnounce(destHash, identAny, appData, hops, isPathResponse)
}

func (h *propagationAnnounceHandler) receivedAnnounce(destHash []byte, identAny any, appData []byte, hops uint8, isPathResponse bool) error {
	if h.router == nil || !PNAnnounceDataIsValid(appData) {
		return nil
	}
	id, err := identityFromAnnounce(identAny, destHash)
	if err != nil || id == nil {
		return nil
	}
	expected, err := PropagationDestinationHash(id)
	if err != nil || !bytes.Equal(expected, destHash) {
		return nil
	}
	if h.router.isOwnPropagationHash(destHash) {
		return nil
	}

	arr, err := decodePNAnnounceArray(appData)
	if err != nil || len(arr) < 7 {
		return nil
	}
	timebase, _ := asInt64(arr[1])
	enabled, _ := arr[2].(bool)
	// Upstream uses None for unknown limits and costs, decoded here as
	// -1 so the peer sync gate treats them as unset.
	transferLimit := int64(-1)
	syncLimit := int64(-1)
	if v, ok := asInt64(arr[3]); ok {
		transferLimit = v
	}
	if v, ok := asInt64(arr[4]); ok {
		syncLimit = v
	}
	costs, _ := arr[5].([]any)
	if len(costs) < 3 {
		return nil
	}
	stampCost := int64(-1)
	stampFlex := int64(-1)
	peeringCost := int64(-1)
	if v, ok := asInt64(costs[0]); ok {
		stampCost = v
	}
	if v, ok := asInt64(costs[1]); ok {
		stampFlex = v
	}
	if v, ok := asInt64(costs[2]); ok {
		peeringCost = v
	}
	metadata := map[byte]any{}
	if md, ok := arr[6].(map[any]any); ok {
		metadata = mapAnyToByteKey(md)
	} else if md, ok := arr[6].(map[string]any); ok {
		for k, v := range md {
			if len(k) == 1 {
				metadata[k[0]] = v
			}
		}
	}

	// Upstream only processes peering announces while running as a
	// propagation node.
	if !h.router.propagationEnabled {
		return nil
	}

	cfg := h.router.cfg.Propagation
	if h.router.isStaticPeer(destHash) {
		// Upstream applies static peer updates from regular announces,
		// and from path responses only while the peer has never been
		// heard from.
		if !isPathResponse || h.router.staticPeerNeverHeard(destHash) {
			h.router.peer(destHash, timebase, float64(transferLimit), float64(syncLimit), int(stampCost), int(stampFlex), int(peeringCost), metadata)
		}
		return nil
	}

	tr := h.router.transport
	if cfg.Autopeer && !isPathResponse {
		if enabled {
			if tr != nil && int(tr.HopsTo(destHash)) <= cfg.AutopeerMaxDepth {
				h.router.peer(destHash, timebase, float64(transferLimit), float64(syncLimit), int(stampCost), int(stampFlex), int(peeringCost), metadata)
			} else {
				// Upstream breaks peering when a peer moves outside
				// the auto-peering range.
				h.router.peersMu.RLock()
				_, isPeer := h.router.peers[peerKey(destHash)]
				h.router.peersMu.RUnlock()
				if isPeer {
					h.router.unpeer(destHash, timebase)
				}
			}
		} else {
			h.router.unpeer(destHash, timebase)
		}
	}
	return nil
}

// staticPeerNeverHeard reports whether a static peer has not announced
// itself yet, which is when upstream lets path responses through.
func (r *Router) staticPeerNeverHeard(destHash []byte) bool {
	r.peersMu.RLock()
	defer r.peersMu.RUnlock()
	peer, ok := r.peers[peerKey(destHash)]
	return !ok || peer.LastHeard == 0
}

func (r *Router) announceDelivery() {
	dest := r.DeliveryDestination()
	if dest == nil {
		return
	}
	name := r.cfg.LXMF.DisplayName
	// Upstream announces [display_name, stamp_cost, [SF_COMPRESSION]].
	var nameElem, costElem any
	if name != "" {
		nameElem = []byte(name)
	}
	r.mu.RLock()
	if r.inboundStampCost != nil && *r.inboundStampCost > 0 && *r.inboundStampCost < 255 {
		costElem = int64(*r.inboundStampCost)
	}
	r.mu.RUnlock()
	appData, err := marshalAnnounceAppData([]any{nameElem, costElem, []any{int64(SFCompression)}})
	if err != nil {
		return
	}
	_ = dest.Announce(true, appData, nil)
}

func (r *Router) announcePropagationNode() {
	dest := r.PropagationDestination()
	if dest == nil {
		return
	}
	time.Sleep(nodeAnnounceDelay)
	appData, err := r.propagationAnnounceAppData()
	if err != nil {
		return
	}
	dest.SetDefaultAppData(appData)
	_ = dest.Announce(true, appData, nil)
	if r.controlDest != nil && len(r.controlAllow) > 1 {
		_ = r.controlDest.Announce(true, nil, nil)
	}
}

const nodeAnnounceDelay = 20 * time.Second

// decodePNAnnouncePayload unpacks propagation announce fields for peering.
func decodePNAnnouncePayload(appData []byte) (timebase int64, enabled bool, transfer, sync, stampCost, stampFlex, peeringCost int64, metadata map[byte]any, ok bool) {
	arr, err := decodePNAnnounceArray(appData)
	if err != nil || len(arr) < 7 {
		return
	}
	timebase, _ = asInt64(arr[1])
	enabled, _ = arr[2].(bool)
	transfer = -1
	sync = -1
	stampCost = -1
	stampFlex = -1
	peeringCost = -1
	if v, vok := asInt64(arr[3]); vok {
		transfer = v
	}
	if v, vok := asInt64(arr[4]); vok {
		sync = v
	}
	costs, _ := arr[5].([]any)
	if len(costs) < 3 {
		return
	}
	if v, vok := asInt64(costs[0]); vok {
		stampCost = v
	}
	if v, vok := asInt64(costs[1]); vok {
		stampFlex = v
	}
	if v, vok := asInt64(costs[2]); vok {
		peeringCost = v
	}
	if md, mOk := arr[6].(map[any]any); mOk {
		metadata = mapAnyToByteKey(md)
	}
	ok = true
	return
}

func recallAppData(destHash []byte) []byte {
	data, ok := identity.GetKnownDestination(hex.EncodeToString(destHash))
	if !ok || len(data) < 4 {
		return nil
	}
	appData, _ := data[3].([]byte)
	return appData
}

func (r *Router) tryAutopeerFromIncomingSync(remotePropHash []byte) {
	if remotePropHash == nil || r.isOwnPropagationHash(remotePropHash) {
		return
	}
	r.peersMu.RLock()
	_, known := r.peers[peerKey(remotePropHash)]
	r.peersMu.RUnlock()
	if known {
		return
	}
	appData := recallAppData(remotePropHash)
	if !PNAnnounceDataIsValid(appData) {
		return
	}
	timebase, enabled, transfer, sync, stampCost, stampFlex, peeringCost, metadata, ok := decodePNAnnouncePayload(appData)
	if !ok {
		return
	}
	cfg := r.cfg.Propagation
	if !cfg.Autopeer || !enabled || r.transport == nil {
		return
	}
	if int(r.transport.HopsTo(remotePropHash)) > cfg.AutopeerMaxDepth {
		return
	}
	Verbose("autopeer from incoming sync", "peer", hex.EncodeToString(remotePropHash))
	r.peer(remotePropHash, timebase, float64(transfer), float64(sync), int(stampCost), int(stampFlex), int(peeringCost), metadata)
}
