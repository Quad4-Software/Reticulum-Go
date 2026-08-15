// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
)

const (
	lrProofX25519Size = 32
	lrProofMTUSize    = 3
)

// validateAndForwardLRProof cryptographically validates a transit link-request
// proof before forwarding it toward the initiator (Python Transport LRPROOF path).
// When ALLOW_LINK_PATH_REBALANCE is on, a hop mismatch on the correct next-hop
// iface may update RemainingHops and path hops after signature validation
// (RNS 1.4.1), then continue with normal forward when hops match.
func (t *Transport) validateAndForwardLRProof(pkt *packet.Packet, iface common.NetworkInterface) bool {
	if t == nil || t.linkTable == nil || pkt == nil {
		return false
	}

	linkID := pkt.DestinationHash
	if len(linkID) > packet.TruncatedHashLength {
		linkID = linkID[:packet.TruncatedHashLength]
	}
	entry, ok := t.linkTable.get(linkID)
	if !ok || entry == nil {
		return false
	}
	forLocalClient := isLocalClientInterface(entry.ReceivedIface)
	if !t.transportEnabled() && !isLocalClientInterface(iface) && !forLocalClient {
		return false
	}

	accounted := AccountInboundHops(pkt.Hops, iface)
	if int(accounted) != entry.RemainingHops {
		if !t.tryTransitLRProofRebalance(pkt, iface, linkID, entry, accounted) {
			ifaceName := ""
			if iface != nil {
				ifaceName = iface.GetName()
			}
			health.Inc(ifaceName, health.KindLRProofHopMismatch)
			debug.Log(debug.DebugInfo, "LRPROOF hop mismatch, not transporting",
				"link_id", fmt.Sprintf("%x", linkID),
				"accounted", accounted,
				"remaining", entry.RemainingHops)
			return true
		}
		entry, ok = t.linkTable.get(linkID)
		if !ok || entry == nil || int(accounted) != entry.RemainingHops {
			return true
		}
	}
	if iface != entry.NextHopIface {
		debug.Log(debug.DebugInfo, "LRPROOF received on wrong interface, not transporting",
			"link_id", fmt.Sprintf("%x", linkID))
		return true
	}

	sigLen := identity.SigLength / 8
	minLen := sigLen + lrProofX25519Size
	maxLen := minLen + lrProofMTUSize
	if len(pkt.Data) != minLen && len(pkt.Data) != maxLen {
		debug.Log(debug.DebugInfo, "LRPROOF data length invalid for transport",
			"link_id", fmt.Sprintf("%x", linkID),
			"data_len", len(pkt.Data))
		return true
	}

	peerIdent, err := identity.Recall(entry.DestinationHash)
	if err != nil || peerIdent == nil {
		debug.Log(debug.DebugInfo, "LRPROOF cannot recall destination identity",
			"dest_hash", fmt.Sprintf("%x", entry.DestinationHash),
			"error", err)
		return true
	}
	pub := peerIdent.GetPublicKey()
	if len(pub) < lrProofX25519Size*2 {
		debug.Log(debug.DebugInfo, "LRPROOF recalled identity key too short")
		return true
	}

	signature := pkt.Data[:sigLen]
	peerPub := pkt.Data[sigLen : sigLen+lrProofX25519Size]
	signalling := []byte(nil)
	if len(pkt.Data) == maxLen {
		signalling = pkt.Data[sigLen+lrProofX25519Size:]
	}
	peerSigPub := pub[lrProofX25519Size : lrProofX25519Size*2]

	signed := make([]byte, 0, len(linkID)+len(peerPub)+len(peerSigPub)+len(signalling))
	signed = append(signed, linkID...)
	signed = append(signed, peerPub...)
	signed = append(signed, peerSigPub...)
	signed = append(signed, signalling...)

	if !peerIdent.Verify(signed, signature) {
		debug.Log(debug.DebugInfo, "Invalid LRPROOF signature in transport, dropping",
			"link_id", fmt.Sprintf("%x", linkID))
		return true
	}

	if !t.linkTable.markValidated(linkID) {
		return true
	}

	if entry.ReceivedIface == nil || !entry.ReceivedIface.IsEnabled() {
		return true
	}
	out := rewriteHopsOnly(pkt.Raw, accounted)
	debug.Log(debug.DebugInfo, "LRPROOF validated for transport",
		"link_id", fmt.Sprintf("%x", linkID),
		"out_iface", entry.ReceivedIface.GetName())
	if err := sendOnInterface(entry.ReceivedIface, out, ""); err != nil {
		debug.Log(debug.DebugError, "Failed to transport LRPROOF", "error", err)
	}
	t.rememberPacketHashForced(pkt)
	t.noteIfacePathSuccess(iface)
	return true
}

// tryTransitLRProofRebalance attempts RNS 1.4.1 hop correction on a transit
// LRPROOF. Requires correct next-hop iface, valid signature, and an
// unvalidated link-table entry. Returns true when RemainingHops was updated.
func (t *Transport) tryTransitLRProofRebalance(pkt *packet.Packet, iface common.NetworkInterface, linkID []byte, entry *LinkRelayEntry, accounted byte) bool {
	if t == nil || pkt == nil || entry == nil || !t.LinkPathRebalanceAllowed() {
		return false
	}
	if iface != entry.NextHopIface {
		return false
	}
	if entry.Validated {
		return false
	}
	sigLen := identity.SigLength / 8
	minLen := sigLen + lrProofX25519Size
	maxLen := minLen + lrProofMTUSize
	if len(pkt.Data) != minLen && len(pkt.Data) != maxLen {
		return false
	}
	peerIdent, err := identity.Recall(entry.DestinationHash)
	if err != nil || peerIdent == nil {
		return false
	}
	pub := peerIdent.GetPublicKey()
	if len(pub) < lrProofX25519Size*2 {
		return false
	}
	signature := pkt.Data[:sigLen]
	peerPub := pkt.Data[sigLen : sigLen+lrProofX25519Size]
	signalling := []byte(nil)
	if len(pkt.Data) == maxLen {
		signalling = pkt.Data[sigLen+lrProofX25519Size:]
	}
	peerSigPub := pub[lrProofX25519Size : lrProofX25519Size*2]
	signed := make([]byte, 0, len(linkID)+len(peerPub)+len(peerSigPub)+len(signalling))
	signed = append(signed, linkID...)
	signed = append(signed, peerPub...)
	signed = append(signed, peerSigPub...)
	signed = append(signed, signalling...)
	if !peerIdent.Verify(signed, signature) {
		debug.Log(debug.DebugInfo, "Aborting LRPROOF path rebalance due to invalid signature",
			"link_id", fmt.Sprintf("%x", linkID))
		return false
	}
	if !t.applyPathHopRebalance(entry.DestinationHash, accounted, iface) {
		return false
	}
	if !t.linkTable.setRemainingHops(linkID, int(accounted)) {
		return false
	}
	debug.Log(debug.DebugInfo, "Re-balancing path from transit link-request proof",
		"link_id", fmt.Sprintf("%x", linkID),
		"dest", pathHex(entry.DestinationHash),
		"to", accounted)
	return true
}
