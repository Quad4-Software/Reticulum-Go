// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/cryptography"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/protect"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
)

func HandleIncomingLinkRequest(pkt *packet.Packet, dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface) (*Link, error) {
	startTime := time.Now()
	debug.Log(debug.DebugVerbose, "Creating link for incoming request", "dest_hash", fmt.Sprintf("%x", dest.GetHash()), "interface", networkIface.GetName())

	if transport != nil {
		if !transport.BeginIncomingHandshake() {
			return nil, errors.New("incoming link limit reached")
		}
		defer transport.EndIncomingHandshake()
	}

	ifaceName := ""
	if networkIface != nil {
		ifaceName = networkIface.GetName()
	}
	d, release := protect.AdmitHandshake(ifaceName)
	if !d.Allow {
		return nil, errors.New("dos_protection refused handshake")
	}
	defer release()

	l := NewLink(dest, transport, networkIface, nil, nil)
	l.status.Store(int32(StatusPending))
	l.initiator = false

	if dest.GetLinkCallback() != nil {
		l.SetEstablishedCallback(func(lnk *Link) {
			dest.GetLinkCallback()(lnk)
		})
	}

	ownerIdentity := dest.GetIdentity()
	if ownerIdentity == nil {
		return nil, errors.New("destination has no identity")
	}

	if err := l.HandleLinkRequest(pkt, ownerIdentity); err != nil {
		debug.Log(debug.DebugError, "Failed to handle link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		return nil, err
	}

	go l.startWatchdog()

	debug.Log(debug.DebugInfo, "Link established for incoming request", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return l, nil
}
func (l *Link) Identify(id *identity.Identity) error {
	if !l.IsActive() {
		return common.ErrLinkNotActive
	}

	pubKey := id.GetPublicKey()
	signData := append(l.linkID, pubKey...)
	signature, err := id.Sign(signData)
	if err != nil {
		return fmt.Errorf("sign link identify: %w", err)
	}

	identData := append(pubKey, signature...)

	encrypted, err := l.encrypt(identData)
	if err != nil {
		return err
	}

	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextLinkIdentify,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            encrypted,
		CreateReceipt:   true,
	}

	if err := p.Pack(); err != nil {
		return err
	}

	return l.transport.SendPacket(p)
}
func (l *Link) HandleIdentification(data []byte) error {
	pubKeySize := identity.KeySize / 8
	if len(data) < pubKeySize+cryptography.Ed25519SignatureSize {
		debug.Log(debug.DebugWarning, "Invalid identification data length", "length", len(data))
		return errors.New("invalid identification data length")
	}

	pubKey := data[:pubKeySize]
	signature := data[pubKeySize:]

	debug.Log(debug.DebugVerbose, "Processing identification from public key", "public_key", fmt.Sprintf("%x", pubKey[:8]))

	remoteIdentity := identity.FromPublicKey(pubKey)
	if remoteIdentity == nil {
		debug.Log(debug.DebugWarning, "Invalid remote identity from public key", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		return errors.New("invalid remote identity")
	}

	signData := append(l.linkID, pubKey...)
	if !remoteIdentity.Verify(signData, signature) {
		debug.Log(debug.DebugWarning, "Invalid signature from remote identity", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		return errors.New("invalid signature")
	}

	debug.Log(debug.DebugVerbose, "Remote identity verified successfully", "public_key", fmt.Sprintf("%x", pubKey[:8]))

	if tab := l.transport.BlackholeTable(); tab != nil {
		if tab.Has(remoteIdentity.Hash()) {
			debug.Log(debug.DebugWarning, "Terminating link from blackholed identity",
				"identity", fmt.Sprintf("%x", remoteIdentity.Hash()))
			l.Teardown()
			return errors.New("remote identity is blackholed")
		}
	}

	// Match Python 1.3.9: set remote identity and fire the callback only once.
	l.mutex.Lock()
	if l.remoteIdentity != nil {
		l.mutex.Unlock()
		return nil
	}
	l.remoteIdentity = remoteIdentity
	cb := l.identifiedCallback
	l.mutex.Unlock()
	if cb != nil {
		debug.Log(debug.DebugVerbose, "Executing identified callback for remote identity", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		cb(l, remoteIdentity)
	}

	return nil
}

// bindOwnerSigningKeys replaces ephemeral Ed25519 material with the destination
// owner identity keys (Python responder Link.sig_prv = owner.identity.sig_prv).
func (l *Link) bindOwnerSigningKeys(owner *identity.Identity) error {
	if owner == nil {
		return errors.New("nil owner identity")
	}
	sigPub := owner.GetSigningKey()
	if len(sigPub) != ed25519.PublicKeySize {
		return errors.New("owner identity has invalid signing public key")
	}
	l.sigPub = append([]byte(nil), sigPub...)

	pk, err := owner.GetPrivateKey()
	if err != nil {
		// Hardware-bound identities cannot export seeds. ProvePacket uses
		// Identity.Sign for responders so leave sigPriv empty.
		closeSecBuf(&l.sigPriv)
		l.sigPriv = nil
		return nil
	}
	defer securemem.WipeBytes(pk)
	if len(pk) < 64 {
		return errors.New("owner private key too short")
	}
	expanded := ed25519.NewKeyFromSeed(pk[32:64])
	if err := setSecBuf(&l.sigPriv, []byte(expanded)); err != nil {
		securemem.WipeBytes(expanded)
		return err
	}
	securemem.WipeBytes(expanded)
	return nil
}
func signallingBytes(mtu int, mode byte) []byte {
	bytes := make([]byte, LinkMTUSize)
	bytes[0] = byte((mtu >> 16) & 0xFF)
	bytes[1] = byte((mtu >> 8) & 0xFF)
	bytes[2] = byte(mtu & 0xFF)
	bytes[0] |= (mode << 5)
	return bytes
}

// prepareLinkRequestLocked builds the outbound link request and derives linkID.
// The link mutex must be held by the caller.
func (l *Link) prepareLinkRequestLocked() error {
	if err := l.generateEphemeralKeys(); err != nil {
		return err
	}

	l.mode = ModeDefault
	l.mtu = common.DefaultMTU / 3
	l.updateMDU()

	signalling := signallingBytes(l.mtu, l.mode)
	requestData := make([]byte, 0, ECPubSize+LinkMTUSize)
	requestData = append(requestData, l.pub...)
	requestData = append(requestData, l.sigPub...)
	requestData = append(requestData, signalling...)

	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		TransportType:   0,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: l.destination.GetType(),
		DestinationHash: l.destination.GetHash(),
		Data:            requestData,
		CreateReceipt:   false,
	}

	if err := pkt.Pack(); err != nil {
		return fmt.Errorf("failed to pack link request: %w", err)
	}

	l.linkID = linkIDFromPacket(pkt)
	l.requestPacket = pkt
	l.requestTime = time.Now()
	l.status.Store(int32(StatusPending))
	return nil
}
func (l *Link) sendPreparedLinkRequest() error {
	pkt := l.requestPacket
	if pkt == nil {
		return errors.New("link request not prepared")
	}
	if l.transport == nil {
		return errors.New("transport is nil")
	}

	sendStartTime := time.Now()
	if err := l.transport.SendPacket(pkt); err != nil {
		debug.Log(debug.DebugError, "Failed to send link request", "error", err, "elapsed", time.Since(sendStartTime).Seconds())
		return fmt.Errorf("failed to send link request: %w", err)
	}

	debug.Log(debug.DebugVerbose, "Link request sent", "link_id", fmt.Sprintf("%x", l.linkID), "send_elapsed", time.Since(sendStartTime).Seconds(), "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	return nil
}
func linkIDFromPacket(pkt *packet.Packet) []byte {
	return packet.LinkIDFromLinkRequest(pkt)
}
func (l *Link) HandleLinkRequest(pkt *packet.Packet, ownerIdentity *identity.Identity) error {
	startTime := time.Now()
	debug.Log(debug.DebugVerbose, "Handling incoming link request", "data_len", len(pkt.Data), "has_interface", l.networkInterface != nil, "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	if len(pkt.Data) < ECPubSize {
		return errors.New("link request data too short")
	}

	peerPub := pkt.Data[0:KeySize]
	peerSigPub := pkt.Data[KeySize:ECPubSize]

	l.peerPub = append([]byte(nil), peerPub...)
	l.peerSigPub = append([]byte(nil), peerSigPub...)
	l.linkID = linkIDFromPacket(pkt)
	l.initiator = false

	myPubStr := "not_generated_yet"
	if len(l.pub) >= 8 {
		myPubStr = fmt.Sprintf("%x", l.pub[:8])
	}
	debug.Log(debug.DebugVerbose, "Link request processed (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "my_pub", myPubStr, "elapsed", time.Since(startTime).Seconds())

	if len(pkt.Data) >= ECPubSize+LinkMTUSize {
		mtuBytes := pkt.Data[ECPubSize : ECPubSize+LinkMTUSize]
		l.mtu = (int(mtuBytes[0]&0x1F) << 16) | (int(mtuBytes[1]) << 8) | int(mtuBytes[2])
		l.mode = (mtuBytes[0] & ModeByteMask) >> 5
		debug.Log(debug.DebugVerbose, "Link request includes MTU", "mtu", l.mtu, "mode", l.mode)
	} else {
		l.mtu = common.DefaultMTU / 3
		l.mode = ModeDefault
	}

	if err := l.generateEphemeralKeys(); err != nil {
		return err
	}
	// Python responders use owner.identity.sig_prv for link signing. Keep the
	// ephemeral X25519 key from generateEphemeralKeys but bind Ed25519 to the
	// destination identity so DATA proofs validate on NomadNet/Python.
	if err := l.bindOwnerSigningKeys(ownerIdentity); err != nil {
		return err
	}

	debug.Log(debug.DebugVerbose, "Ephemeral keys generated (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "my_pub", fmt.Sprintf("%x", l.pub[:8]), "peer_pub", fmt.Sprintf("%x", l.peerPub[:8]))

	if err := l.performHandshake(); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	l.status.Store(int32(StatusHandshake))
	l.recordInbound(false)
	l.requestTime = time.Now()
	// Establishment timeout is per-hop plus keepalive grace so WAN and
	// backbone proof or RTT races are not closed too aggressively.
	hops := max(int(pkt.Hops), 1)
	l.establishmentTimeout = time.Duration(float64(hops)*EstablishmentTimeoutPerHop*float64(time.Second)) + l.keepalive
	debug.Log(debug.DebugVerbose, "Responder establishment timeout configured", "link_id", fmt.Sprintf("%x", l.linkID), "packet_hops", pkt.Hops, "effective_hops", hops, "timeout_sec", l.establishmentTimeout.Seconds())

	// Register before sending proof so an immediate LRRTT cannot race and miss.
	if l.transport != nil {
		l.transport.RegisterLink(l.linkID, l)
		if l.networkInterface != nil {
			l.registerLinkPath()
		}
	}

	proofStartTime := time.Now()
	if err := l.sendLinkProof(ownerIdentity); err != nil {
		debug.Log(debug.DebugError, "Failed to send link proof", "error", err, "elapsed", time.Since(proofStartTime).Seconds())
		return fmt.Errorf("failed to send link proof: %w", err)
	}

	debug.Log(debug.DebugVerbose, "Link proof sent (responder), waiting for RTT", "link_id", fmt.Sprintf("%x", l.linkID), "proof_send_elapsed", time.Since(proofStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())

	return nil
}
func (l *Link) updateMDU() {
	headerMinSize := 19
	ifacMinSize := 1
	tokenOverhead := common.TokenOverhead
	aesBlockSize := 16

	if l.mtu > packet.MTU {
		debug.Log(debug.DebugVerbose, "Clamping negotiated link MTU to packet.MTU", "negotiated", l.mtu, "packet_mtu", packet.MTU)
		l.mtu = packet.MTU
	}

	l.mdu = int(float64(l.mtu-headerMinSize-ifacMinSize-tokenOverhead)/float64(aesBlockSize))*aesBlockSize - 1
	if l.mdu < 0 {
		l.mdu = common.DefaultMTU / 15
	}
}
func (l *Link) resourceSDU() int {
	resourceHeaderMaxSize := 35
	resourceIFACMinSize := 1

	l.mutex.RLock()
	mtu := l.mtu
	mdu := l.mdu
	l.mutex.RUnlock()

	if mtu > 0 {
		sdu := mtu - resourceHeaderMaxSize - resourceIFACMinSize
		if sdu > 0 {
			return sdu
		}
	}

	return mdu
}
func (l *Link) performHandshake() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.performHandshakeLocked()
}
func (l *Link) performHandshakeLocked() error {
	if len(l.peerPub) != KeySize {
		return errors.New("invalid peer public key length")
	}
	if l.prv == nil {
		return errors.New("missing ephemeral private key")
	}

	sharedSecret, err := cryptography.DeriveSharedSecret(l.prv.Bytes(), l.peerPub)
	if err != nil {
		return fmt.Errorf("ECDH failed: %w", err)
	}
	if err := setSecBuf(&l.sharedKey, sharedSecret); err != nil {
		securemem.WipeBytes(sharedSecret)
		return err
	}
	securemem.WipeBytes(sharedSecret)

	var derivedKeyLength int
	if l.mode == ModeAES128CBC {
		derivedKeyLength = 32
	} else if l.mode == ModeAES256CBC {
		derivedKeyLength = 64
	} else {
		return fmt.Errorf("invalid link mode: %d", l.mode)
	}

	derivedKey, err := cryptography.DeriveKey(l.sharedKey.Bytes(), l.linkID, nil, derivedKeyLength)
	if err != nil {
		return fmt.Errorf("HKDF failed: %w", err)
	}
	if err := setSecBuf(&l.derivedKey, derivedKey); err != nil {
		securemem.WipeBytes(derivedKey)
		return err
	}

	if len(derivedKey) >= 64 {
		if err := setSecBuf(&l.hmacKey, derivedKey[0:32]); err != nil {
			securemem.WipeBytes(derivedKey)
			return err
		}
		if err := setSecBuf(&l.sessionKey, derivedKey[32:64]); err != nil {
			securemem.WipeBytes(derivedKey)
			return err
		}
		debug.Log(debug.DebugVerbose, "Session keys derived", "link_id", fmt.Sprintf("%x", l.linkID), "mode", l.mode, "initiator", l.initiator, "key_material_bytes", len(derivedKey))
	} else if len(derivedKey) >= 32 {
		if err := setSecBuf(&l.hmacKey, derivedKey[0:16]); err != nil {
			securemem.WipeBytes(derivedKey)
			return err
		}
		if err := setSecBuf(&l.sessionKey, derivedKey[16:32]); err != nil {
			securemem.WipeBytes(derivedKey)
			return err
		}
	}
	securemem.WipeBytes(derivedKey)
	l.refreshAESBlockLocked()

	l.status.Store(int32(StatusHandshake))
	debug.Log(debug.DebugVerbose, "Handshake completed", "key_material_bytes", derivedKeyLength, "link_id", fmt.Sprintf("%x", l.linkID))
	return nil
}
