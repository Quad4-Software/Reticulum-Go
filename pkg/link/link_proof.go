// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/protect"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func (l *Link) handleLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	if !l.initiator {
		return nil
	}
	return l.validateLinkProofLocked(pkt, networkIface)
}
func incProofFail(networkIface common.NetworkInterface) {
	ifaceName := ""
	if networkIface != nil {
		ifaceName = networkIface.GetName()
	}
	health.Inc(ifaceName, health.KindProofFail)
}
func (l *Link) SetProofStrategy(strategy byte) error {
	if strategy != ProveNone && strategy != ProveAll && strategy != ProveApp {
		return errors.New("invalid proof strategy")
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.proofStrategy = strategy
	return nil
}
func (l *Link) SetProofCallback(callback func(*packet.Packet) bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.proofCallback = callback
}

// maybeProveInboundData mirrors Python Link DATA handling: when the attached
// destination asks for ProveAll (or ProveApp via callback), send a link proof
// so the sender can mark LXMF delivery as DELIVERED.
func (l *Link) maybeProveInboundData(pkt *packet.Packet) {
	if pkt == nil {
		return
	}
	l.mutex.RLock()
	dest := l.destination
	linkProofCB := l.proofCallback
	l.mutex.RUnlock()
	if dest == nil {
		return
	}
	switch dest.ProofStrategy() {
	case destination.ProveAll:
		if err := l.ProvePacket(pkt); err != nil {
			debug.Log(debug.DebugWarning, "Failed to prove inbound link packet", "error", err)
		}
	case destination.ProveApp:
		if linkProofCB != nil && linkProofCB(pkt) {
			if err := l.ProvePacket(pkt); err != nil {
				debug.Log(debug.DebugWarning, "Failed to prove inbound link packet", "error", err)
			}
		}
	}
}

// ProvePacket sends an explicit link proof for pkt (Python Link.prove_packet).
// Responder links must sign with the destination identity key. Python
// initiators validate via peer_sig_pub from the destination identity, not the
// ephemeral Ed25519 keypair used only by initiators.
func (l *Link) ProvePacket(pkt *packet.Packet) error {
	if pkt == nil {
		return errors.New("nil packet")
	}
	if !l.IsActive() {
		return common.ErrLinkNotActive
	}
	hash := pkt.GetHash()
	if len(hash) == 0 {
		return errors.New("empty packet hash")
	}

	l.mutex.RLock()
	initiator := l.initiator
	dest := l.destination
	sigPriv := l.sigPriv
	linkID := append([]byte(nil), l.linkID...)
	l.mutex.RUnlock()

	var signature []byte
	if !initiator && dest != nil && dest.GetIdentity() != nil {
		sig, err := dest.GetIdentity().Sign(hash)
		if err != nil {
			return err
		}
		signature = sig
	} else {
		if sigPriv == nil || sigPriv.Len() == 0 {
			return errors.New("link has no signing key")
		}
		priv := sigPriv.CopyOut()
		defer securemem.WipeBytes(priv)
		signature = ed25519.Sign(ed25519.PrivateKey(priv), hash)
	}
	proofData := append(append([]byte(nil), hash...), signature...)

	proofPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeProof,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: linkID,
		Data:            proofData,
		CreateReceipt:   false,
	}
	if err := proofPkt.Pack(); err != nil {
		return err
	}
	l.recordOutbound()
	return l.transport.SendPacket(proofPkt)
}
func (l *Link) HandleProofRequest(packet *packet.Packet) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// Callers that receive proof-request contexts should consult this before sending a proof.
	switch l.proofStrategy {
	case ProveNone:
		return false
	case ProveAll:
		return true
	case ProveApp:
		if l.proofCallback != nil {
			return l.proofCallback(packet)
		}
		return false
	default:
		return false
	}
}
func (l *Link) sendLinkProof(ownerIdentity *identity.Identity) error {
	debug.Log(debug.DebugVerbose, "Generating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "initiator", l.initiator, "has_interface", l.networkInterface != nil)

	proofPkt, err := l.GenerateLinkProof(ownerIdentity)
	if err != nil {
		return err
	}

	debug.Log(debug.DebugVerbose, "Link proof packet created", "dest_hash", fmt.Sprintf("%x", proofPkt.DestinationHash), "packet_type", fmt.Sprintf("0x%02x", proofPkt.PacketType))

	// For responder links (not initiator), send proof directly through the receiving interface
	if !l.initiator && l.networkInterface != nil {
		if err := proofPkt.Pack(); err != nil {
			return fmt.Errorf("failed to pack proof packet: %w", err)
		}

		debug.Log(debug.DebugVerbose, "Sending proof through interface", "raw_len", len(proofPkt.Raw), "interface", l.networkInterface.GetName())

		if err := l.networkInterface.Send(proofPkt.Raw, ""); err != nil {
			return fmt.Errorf("failed to send link proof through interface: %w", err)
		}
		debug.Log(debug.DebugVerbose, "Link proof sent through interface", "link_id", fmt.Sprintf("%x", l.linkID), "interface", l.networkInterface.GetName())
		return nil
	}

	// For initiator links, use transport (path lookup)
	if l.transport != nil {
		if err := l.transport.SendPacket(proofPkt); err != nil {
			return fmt.Errorf("failed to send link proof: %w", err)
		}
		debug.Log(debug.DebugVerbose, "Link proof sent", "link_id", fmt.Sprintf("%x", l.linkID))
	}

	return nil
}
func (l *Link) GenerateLinkProof(ownerIdentity *identity.Identity) (*packet.Packet, error) {
	signalling := signallingBytes(l.mtu, l.mode)

	ownerSigPub := ownerIdentity.GetPublicKey()[KeySize:ECPubSize]

	signedData := make([]byte, 0, len(l.linkID)+KeySize+len(ownerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, l.pub...)
	signedData = append(signedData, ownerSigPub...)
	signedData = append(signedData, signalling...)

	signature, err := ownerIdentity.Sign(signedData)
	if err != nil {
		return nil, fmt.Errorf("sign link proof: %w", err)
	}
	debug.Log(
		debug.DebugInfo,
		"Generated link proof signature",
		"link_id", fmt.Sprintf("%x", l.linkID),
		"sig_prefix", fmt.Sprintf("%x", signature[:8]),
		"pub_prefix", fmt.Sprintf("%x", l.pub[:8]),
		"owner_sig_pub_prefix", fmt.Sprintf("%x", ownerSigPub[:8]),
		"signalling", fmt.Sprintf("%x", signalling),
	)

	proofData := make([]byte, 0, len(signature)+KeySize+len(signalling))
	proofData = append(proofData, signature...)
	proofData = append(proofData, l.pub...)
	proofData = append(proofData, signalling...)

	proofPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeProof,
		TransportType:   0,
		Context:         packet.ContextLRProof,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            proofData,
		CreateReceipt:   false,
		Link:            l,
	}

	if err := proofPkt.Pack(); err != nil {
		return nil, fmt.Errorf("failed to pack link proof: %w", err)
	}

	return proofPkt, nil
}
func (l *Link) ValidateLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	ifaceName := ""
	if networkIface != nil {
		ifaceName = networkIface.GetName()
	}
	d, release := protect.AdmitHandshake(ifaceName)
	if !d.Allow {
		return errors.New("dos_protection refused handshake")
	}
	defer release()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.validateLinkProofLocked(pkt, networkIface)
}

// validateLinkProofLocked completes initiator-side link establishment after
// receiving the responder's signed proof. The link mutex must be held.
func (l *Link) validateLinkProofLocked(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	if !l.initiator {
		// Responders never validate LRProofs. Without this gate a reflected
		// copy of our own signed proof verifies against the local identity,
		// overwrites peerPub with our own ephemeral key, and fires the
		// established callback on a bogus self-session.
		return nil
	}
	startTime := time.Now()
	debug.Log(debug.DebugVerbose, "Validating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load(), "initiator", l.initiator, "has_interface", networkIface != nil, "proof_data_len", len(pkt.Data))
	st := l.status.Load()
	if st != int32(StatusPending) && st != int32(StatusHandshake) {
		return fmt.Errorf("invalid link status for proof validation: %d", l.status.Load())
	}

	// Match Python Transport pending-link LRPROOF hop gate (RNS 1.3.8 / 1.4.1).
	// Compare accounted inbound hops (wire+1, except local-client ifaces)
	// against expected_hops from the path table. PATHFINDER_M means hops
	// were unknown at link creation. On mismatch while PENDING, a validated
	// LRPROOF may rebalance expected hops once (ALLOW_LINK_PATH_REBALANCE).
	accounted := transport.AccountInboundHops(pkt.Hops, networkIface)
	if l.expectedHops != transport.PathfinderM && accounted != l.expectedHops {
		if !l.tryTerminusPathRebalanceLocked(pkt, networkIface, accounted) {
			// A malformed or forged proof must not kill the pending link.
			// Anyone who saw the broadcast link request can emit one.
			// Python drops it and lets the establishment timeout decide.
			ifaceName := ""
			if networkIface != nil {
				ifaceName = networkIface.GetName()
			}
			health.Inc(ifaceName, health.KindLRProofHopMismatch)
			health.Inc(ifaceName, health.KindProofFail)
			return fmt.Errorf("link proof hop count mismatch: got %d want %d", accounted, l.expectedHops)
		}
	}

	if len(pkt.Data) < identity.SigLength/8+KeySize {
		incProofFail(networkIface)
		return errors.New("link proof data too short")
	}

	signature := pkt.Data[0 : identity.SigLength/8]
	peerPub := pkt.Data[identity.SigLength/8 : identity.SigLength/8+KeySize]

	signalling := []byte{0, 0, 0}
	if len(pkt.Data) >= identity.SigLength/8+KeySize+LinkMTUSize {
		signalling = pkt.Data[identity.SigLength/8+KeySize : identity.SigLength/8+KeySize+LinkMTUSize]
		mtu := (int(signalling[0]&0x1F) << 16) | (int(signalling[1]) << 8) | int(signalling[2])
		mode := (signalling[0] & ModeByteMask) >> 5
		l.mtu = mtu
		l.mode = mode
		debug.Log(debug.DebugVerbose, "Link proof includes MTU", "mtu", mtu, "mode", mode)
	}

	l.peerPub = append([]byte(nil), peerPub...)
	if l.destination != nil && l.destination.GetIdentity() != nil {
		destIdent := l.destination.GetIdentity()
		pubKey := destIdent.GetPublicKey()
		if len(pubKey) >= ECPubSize {
			l.peerSigPub = append([]byte(nil), pubKey[KeySize:ECPubSize]...)
		}
	}

	signedData := make([]byte, 0, len(l.linkID)+KeySize+len(l.peerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, peerPub...)
	signedData = append(signedData, l.peerSigPub...)
	signedData = append(signedData, signalling...)

	first32Len := min(len(signedData), 32)
	debug.Log(debug.DebugVerbose, "Constructed signed data for validation", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "peer_sig_pub", fmt.Sprintf("%x", l.peerSigPub[:8]), "signalling", fmt.Sprintf("%x", signalling), "signed_data_len", len(signedData), "signed_data_first32", fmt.Sprintf("%x", signedData[:first32Len]))

	if l.destination == nil || l.destination.GetIdentity() == nil {
		l.markInitiatorEstablishmentFailedLocked()
		return errors.New("no destination identity for proof validation")
	}

	if !l.destination.GetIdentity().Verify(signedData, signature) {
		debug.Log(debug.DebugError, "Link proof signature validation failed", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "signature", fmt.Sprintf("%x", signature[:8]), "signed_data", fmt.Sprintf("%x", signedData))
		incProofFail(networkIface)
		return errors.New("link proof signature validation failed")
	}
	debug.Log(debug.DebugVerbose, "Link proof signature validated successfully", "link_id", fmt.Sprintf("%x", l.linkID[:8]))

	if err := l.performHandshakeLocked(); err != nil {
		l.markInitiatorEstablishmentFailedLocked()
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	l.rtt = time.Since(l.requestTime).Seconds()
	l.establishedAt = time.Now()
	if l.rtt > 0 {
		l.updateKeepaliveLocked()
	}
	logRtt := l.rtt

	if !l.promoteToActive() {
		l.markInitiatorEstablishmentFailedLocked()
		return errors.New("link closed before becoming active")
	}

	rttData, err := msgpack.Marshal(logRtt)
	if err != nil {
		return fmt.Errorf("failed to encode RTT payload: %w", err)
	}
	rttPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextLRRTT,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            rttData,
		CreateReceipt:   false,
	}
	if l.transport != nil {
		l.transport.RegisterLink(l.linkID, l)
		if l.networkInterface != nil {
			l.registerLinkPath()
		}
	}

	encrypted, err := l.encryptLocked(rttData)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to encrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
	} else {
		rttPkt.Data = encrypted
		if err := rttPkt.Pack(); err != nil {
			debug.Log(debug.DebugError, "Failed to pack RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
		} else {
			debug.Log(debug.DebugVerbose, "Sending RTT packet", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt), "packet_size", len(rttPkt.Raw))
			if err := l.transport.SendPacket(rttPkt); err != nil {
				debug.Log(debug.DebugError, "Failed to send RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			} else {
				l.recordOutbound()
				debug.Log(debug.DebugVerbose, "RTT packet sent successfully", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt))
			}
		}
	}

	establishmentElapsed := time.Since(l.requestTime).Seconds()
	debug.Log(debug.DebugInfo, "Link established (initiator)", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt), "total_elapsed", fmt.Sprintf("%.3fs", establishmentElapsed), "validation_elapsed", fmt.Sprintf("%.3fs", time.Since(startTime).Seconds()))

	if l.establishedCallback != nil {
		// Initiator callback runs from ValidateLinkProof while the link mutex is
		// held. Callbacks call GetLinkID and must not run on this goroutine.
		cb := l.establishedCallback
		go func() {
			cb(l)
			l.flushEarlyChannel()
		}()
	}

	return nil
}
