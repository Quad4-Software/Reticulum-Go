// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package link

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/channel"
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/destination"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
	"git.quad4.io/Networks/Reticulum-Go/pkg/pathfinder"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resolver"
	"git.quad4.io/Networks/Reticulum-Go/pkg/resource"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
	"github.com/vmihailenco/msgpack/v5"
)

func init() {
	destination.RegisterIncomingLinkHandler(func(pkt *packet.Packet, dest *destination.Destination, trans any, networkIface common.NetworkInterface) (any, error) {
		transportObj, ok := trans.(*transport.Transport)
		if !ok {
			return nil, errors.New("invalid transport type")
		}
		return HandleIncomingLinkRequest(pkt, dest, transportObj, networkIface)
	})
}

type Link struct {
	mutex            sync.RWMutex
	destination      *destination.Destination
	status           atomic.Int32
	networkInterface common.NetworkInterface
	establishedAt    time.Time
	lastInbound      time.Time
	lastOutbound     time.Time
	lastDataReceived time.Time
	lastDataSent     time.Time
	pathFinder       *pathfinder.PathFinder

	remoteIdentity *identity.Identity
	sessionKey     []byte
	linkID         []byte

	rtt               float64
	establishmentRate float64

	establishedCallback func(*Link)
	closedCallback      func(*Link)
	packetCallback      func([]byte, *packet.Packet)
	identifiedCallback  func(*Link, *identity.Identity)

	teardownReason byte
	hmacKey        []byte
	transport      *transport.Transport

	rssi                      float64
	snr                       float64
	q                         float64
	resourceCallback          func(any) bool
	resourceStartedCallback   func(any)
	resourceConcludedCallback func(any)
	resourceStrategy          byte
	proofStrategy             byte
	proofCallback             func(*packet.Packet) bool
	trackPhyStats             bool

	watchdogLock         bool
	watchdogActive       bool
	establishmentTimeout time.Duration
	keepalive            time.Duration
	staleTime            time.Duration
	initiator            bool

	prv           []byte
	sigPriv       ed25519.PrivateKey
	pub           []byte
	sigPub        ed25519.PublicKey
	peerPub       []byte
	peerSigPub    ed25519.PublicKey
	sharedKey     []byte
	derivedKey    []byte
	mode          byte
	mtu           int
	mdu           int
	requestTime   time.Time
	requestPacket *packet.Packet

	pendingRequests []*RequestReceipt
	requestMutex    sync.RWMutex

	channel      *channel.Channel
	channelMutex sync.RWMutex
}

func NewLink(dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface, establishedCallback func(*Link), closedCallback func(*Link)) *Link {
	return &Link{
		destination:         dest,
		transport:           transport,
		networkInterface:    networkIface,
		establishedCallback: establishedCallback,
		closedCallback:      closedCallback,
		establishedAt:       time.Time{}, // Zero time until established
		lastInbound:         time.Time{},
		lastOutbound:        time.Time{},
		lastDataReceived:    time.Time{},
		lastDataSent:        time.Time{},
		pathFinder:          pathfinder.NewPathFinder(),

		watchdogLock:         false,
		watchdogActive:       false,
		establishmentTimeout: time.Duration(ESTABLISHMENT_TIMEOUT_PER_HOP * float64(time.Second)),
		keepalive:            time.Duration(KEEPALIVE * float64(time.Second)),
		staleTime:            time.Duration(STALE_TIME * float64(time.Second)),
		initiator:            false,
		pendingRequests:      make([]*RequestReceipt, 0),
	}
}

func HandleIncomingLinkRequest(pkt *packet.Packet, dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface) (*Link, error) {
	startTime := time.Now()
	debug.Log(debug.DEBUG_INFO, "Creating link for incoming request", "dest_hash", fmt.Sprintf("%x", dest.GetHash()), "interface", networkIface.GetName())

	l := NewLink(dest, transport, networkIface, nil, nil)
	l.status.Store(int32(STATUS_PENDING))
	l.initiator = false // This is a responder link

	// Set the established callback from the destination if it exists
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
		debug.Log(debug.DEBUG_ERROR, "Failed to handle link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		return nil, err
	}

	go l.startWatchdog()

	debug.Log(debug.DEBUG_INFO, "Link established for incoming request", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return l, nil
}

func (l *Link) Establish() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	startTime := time.Now()
	debug.Log(debug.DEBUG_INFO, "Establishing link", "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))

	if l.status.Load() != int32(STATUS_PENDING) {
		debug.Log(debug.DEBUG_INFO, "Cannot establish link: invalid status", "status", l.status.Load())
		return errors.New("link already established or failed")
	}

	if l.destination == nil {
		return errors.New("destination is nil")
	}

	l.initiator = true
	l.status.Store(int32(STATUS_PENDING))
	l.requestTime = time.Now()

	if err := l.SendLinkRequest(); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to send link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		return err
	}

	if l.transport != nil {
		l.transport.RegisterLink(l.linkID, l)

		// If network interface is not set, try to find it from transport paths
		if l.networkInterface == nil {
			if ifaceName := l.transport.NextHopInterface(l.destination.GetHash()); ifaceName != "" {
				if iface, err := l.transport.GetInterface(ifaceName); err == nil {
					l.networkInterface = iface
				}
			}
		}

		if l.networkInterface != nil {
			l.transport.UpdatePath(l.linkID, nil, l.networkInterface.GetName(), 0)
		}
	}

	go l.startWatchdog()

	debug.Log(debug.DEBUG_INFO, "Link establishment initiated", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return nil
}

func (l *Link) Identify(id *identity.Identity) error {
	if !l.IsActive() {
		return errors.New("link not active")
	}

	pubKey := id.GetPublicKey()
	signData := append(l.linkID, pubKey...)
	signature := id.Sign(signData)

	identData := append(pubKey, signature...)

	encrypted, err := l.encrypt(identData)
	if err != nil {
		return err
	}

	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextLinkIdentify,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
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
	pubKeySize := identity.KEYSIZE / 8
	if len(data) < pubKeySize+ed25519.SignatureSize {
		debug.Log(debug.DEBUG_INFO, "Invalid identification data length", "length", len(data))
		return errors.New("invalid identification data length")
	}

	pubKey := data[:pubKeySize]
	signature := data[pubKeySize:]

	debug.Log(debug.DEBUG_VERBOSE, "Processing identification from public key", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))

	remoteIdentity := identity.FromPublicKey(pubKey)
	if remoteIdentity == nil {
		debug.Log(debug.DEBUG_INFO, "Invalid remote identity from public key", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
		return errors.New("invalid remote identity")
	}

	signData := append(l.linkID, pubKey...)
	if !remoteIdentity.Verify(signData, signature) {
		debug.Log(debug.DEBUG_INFO, "Invalid signature from remote identity", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
		return errors.New("invalid signature")
	}

	debug.Log(debug.DEBUG_VERBOSE, "Remote identity verified successfully", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
	l.remoteIdentity = remoteIdentity

	if l.identifiedCallback != nil {
		debug.Log(debug.DEBUG_VERBOSE, "Executing identified callback for remote identity", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
		l.identifiedCallback(l, remoteIdentity)
	}

	return nil
}

func (l *Link) Request(path string, data []byte, timeout time.Duration) (*RequestReceipt, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.status.Load() != int32(STATUS_ACTIVE) {
		return nil, errors.New("link not active")
	}

	pathHash := identity.TruncatedHash([]byte(path))
	requestData := []any{time.Now().Unix(), pathHash, data}
	packedRequest, err := msgpack.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to pack request: %w", err)
	}

	if timeout <= 0 {
		timeout = time.Duration(l.rtt*TRAFFIC_TIMEOUT_FACTOR*float64(time.Second)) + time.Duration(resource.RESPONSE_MAX_GRACE_TIME*1.125*float64(time.Second))
	}

	if len(packedRequest) <= l.mdu {
		reqPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   common.ZERO,
			Context:         packet.ContextRequest,
			ContextFlag:     packet.FlagUnset,
			Hops:            common.ZERO,
			DestinationType: DEST_TYPE_LINK,
			DestinationHash: l.linkID,
			Data:            packedRequest,
			CreateReceipt:   false,
		}

		if err := reqPkt.Pack(); err != nil {
			return nil, err
		}

		encrypted, err := l.encrypt(packedRequest)
		if err != nil {
			return nil, err
		}

		reqPkt.Data = encrypted
		reqPkt.Packed = false
		if err := reqPkt.Pack(); err != nil {
			return nil, err
		}

		requestID := reqPkt.TruncatedHash()

		if l.networkInterface != nil {
			debug.Log(debug.DEBUG_INFO, "Sending request through interface", "path", path, "request_id", fmt.Sprintf("%x", requestID), "interface", l.networkInterface.GetName())
			if err := l.networkInterface.Send(reqPkt.Raw, ""); err != nil {
				return nil, fmt.Errorf("failed to send request through interface: %w", err)
			}
		} else {
			if err := l.transport.SendPacket(reqPkt); err != nil {
				return nil, err
			}
		}

		receipt := &RequestReceipt{
			link:      l,
			requestID: requestID,
			status:    STATUS_PENDING,
			sentAt:    time.Now(),
			timeout:   timeout,
		}

		l.requestMutex.Lock()
		l.pendingRequests = append(l.pendingRequests, receipt)
		l.requestMutex.Unlock()

		go receipt.startTimeout()

		return receipt, nil
	}

	return nil, errors.New("request too large, resource transfer not yet implemented")
}

type RequestReceipt struct {
	link       *Link
	mutex      sync.RWMutex
	requestID  []byte
	status     byte
	sentAt     time.Time
	receivedAt time.Time
	response   []byte
	timeout    time.Duration
	responseCb func(*RequestReceipt)
	failedCb   func(*RequestReceipt)
	progressCb func(*RequestReceipt)
}

func (r *RequestReceipt) GetRequestID() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return append([]byte{}, r.requestID...)
}

func (r *RequestReceipt) GetStatus() byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.status
}

func (r *RequestReceipt) GetResponse() []byte {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.response == nil {
		return nil
	}
	return append([]byte{}, r.response...)
}

func (r *RequestReceipt) GetResponseTime() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.receivedAt.IsZero() {
		return common.FLOAT_ZERO
	}
	return r.receivedAt.Sub(r.sentAt).Seconds()
}

func (r *RequestReceipt) Concluded() bool {
	status := r.GetStatus()
	return status == STATUS_ACTIVE || status == STATUS_FAILED
}

func (r *RequestReceipt) startTimeout() {
	time.Sleep(r.timeout)
	r.mutex.Lock()
	if r.status == STATUS_PENDING {
		r.status = STATUS_FAILED
		if r.failedCb != nil {
			go r.failedCb(r)
		}
	}
	r.mutex.Unlock()
}

func (r *RequestReceipt) SetResponseCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.responseCb = cb
}

func (r *RequestReceipt) SetFailedCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.failedCb = cb
}

func (r *RequestReceipt) SetProgressCallback(cb func(*RequestReceipt)) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.progressCb = cb
}

func (l *Link) TrackPhyStats(track bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.trackPhyStats = track
}

func (l *Link) UpdatePhyStats(rssi, snr, q float64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.trackPhyStats {
		l.rssi = rssi
		l.snr = snr
		l.q = q
	}
}

func (l *Link) GetRSSI() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return common.FLOAT_ZERO
	}
	return l.rssi
}

func (l *Link) GetSNR() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return common.FLOAT_ZERO
	}
	return l.snr
}

func (l *Link) GetQ() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return common.FLOAT_ZERO
	}
	return l.q
}

func (l *Link) GetEstablishmentRate() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.establishmentRate
}

func (l *Link) GetAge() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.establishedAt.IsZero() {
		return common.FLOAT_ZERO
	}
	return time.Since(l.establishedAt).Seconds()
}

func (l *Link) NoInboundFor() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.lastInbound.IsZero() {
		return common.FLOAT_ZERO
	}
	return time.Since(l.lastInbound).Seconds()
}

func (l *Link) NoOutboundFor() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.lastOutbound.IsZero() {
		return common.FLOAT_ZERO
	}
	return time.Since(l.lastOutbound).Seconds()
}

func (l *Link) NoDataFor() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	lastData := l.lastDataReceived
	if l.lastDataSent.After(lastData) {
		lastData = l.lastDataSent
	}
	if lastData.IsZero() {
		return common.FLOAT_ZERO
	}
	return time.Since(lastData).Seconds()
}

func (l *Link) InactiveFor() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	lastActivity := l.lastInbound
	if l.lastOutbound.After(lastActivity) {
		lastActivity = l.lastOutbound
	}
	if lastActivity.IsZero() {
		return common.FLOAT_ZERO
	}
	return time.Since(lastActivity).Seconds()
}

func (l *Link) GetRemoteIdentity() *identity.Identity {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.remoteIdentity
}

func (l *Link) Teardown() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.status.Load() == int32(STATUS_ACTIVE) {
		l.status.Store(int32(STATUS_CLOSED))
		if l.closedCallback != nil {
			l.closedCallback(l)
		}
	}
}

func (l *Link) SetEstablishedCallback(callback func(*Link)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.establishedCallback = callback
}

func (l *Link) SetLinkClosedCallback(callback func(*Link)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.closedCallback = callback
}

func (l *Link) SetPacketCallback(callback func([]byte, *packet.Packet)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.packetCallback = callback
}

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

func (l *Link) SetRemoteIdentifiedCallback(callback func(*Link, *identity.Identity)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.identifiedCallback = callback
}

func (l *Link) SetResourceStrategy(strategy byte) error {
	if strategy != ACCEPT_NONE && strategy != ACCEPT_ALL && strategy != ACCEPT_APP {
		return errors.New("unsupported resource strategy")
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.resourceStrategy = strategy
	return nil
}

func (l *Link) SendPacket(data []byte) error {
	return l.SendPacketWithContext(data, packet.ContextNone)
}

func (l *Link) SendPacketWithContext(data []byte, context byte) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.status.Load() != int32(STATUS_ACTIVE) {
		debug.Log(debug.DEBUG_INFO, "Cannot send packet: link not active", "status", l.status.Load())
		return errors.New("link not active")
	}

	debug.Log(debug.DEBUG_VERBOSE, "Encrypting packet", "bytes", len(data), "context", fmt.Sprintf("0x%02x", context))
	var wireData []byte
	var err error
	if context == packet.ContextResource {
		wireData = data
	} else {
		wireData, err = l.encrypt(data)
		if err != nil {
			debug.Log(debug.DEBUG_INFO, "Failed to encrypt packet", "error", err)
			return err
		}
	}

	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         context,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            wireData,
		CreateReceipt:   false,
	}

	if err := p.Pack(); err != nil {
		return err
	}

	debug.Log(debug.DEBUG_VERBOSE, "Sending encrypted packet", "bytes", len(wireData))
	l.lastOutbound = time.Now()
	l.lastDataSent = time.Now()

	return l.transport.SendPacket(p)
}

func (l *Link) HandleInbound(pkt *packet.Packet) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.watchdogLock = true
	defer func() {
		l.watchdogLock = false
	}()

	if l.status.Load() == int32(STATUS_CLOSED) {
		debug.Log(debug.DEBUG_VERBOSE, "Ignoring packet for closed link", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil
	}

	l.lastInbound = time.Now()
	if pkt.Context != packet.ContextKeepalive {
		l.lastDataReceived = time.Now()
	}

	if l.status.Load() == int32(STATUS_STALE) {
		l.status.Store(int32(STATUS_ACTIVE))
	}

	if pkt.PacketType == packet.PacketTypeData {
		return l.handleDataPacket(pkt)
	} else if pkt.PacketType == packet.PacketTypeProof {
		if pkt.Context == packet.ContextLRProof {
			return l.handleLinkProof(pkt, l.networkInterface)
		} else if pkt.Context == packet.ContextLRRTT {
			return l.handleRTTPacket(pkt)
		}
	}

	return nil
}

func (l *Link) handleDataPacket(pkt *packet.Packet) error {
	st := l.status.Load()
	if st != int32(STATUS_ACTIVE) && st != int32(STATUS_HANDSHAKE) {
		return errors.New("link not active")
	}

	if pkt.Context == packet.ContextLRRTT && st == int32(STATUS_HANDSHAKE) && !l.initiator {
		debug.Log(debug.DEBUG_INFO, "RTT packet detected in handleDataPacket, routing to handleRTTPacket", "link_id", fmt.Sprintf("%x", l.linkID))
		return l.handleRTTPacket(pkt)
	}

	var plaintext []byte
	var err error

	if l.sessionKey != nil {
		if pkt.Context == packet.ContextResource {
			plaintext = pkt.Data
		} else {
			plaintext, err = l.decrypt(pkt.Data)
			if err != nil {
				debug.Log(debug.DEBUG_INFO, "Failed to decrypt packet", "error", err, "context", fmt.Sprintf("0x%02x", pkt.Context), "link_id", fmt.Sprintf("%x", l.linkID))
				return err
			}
		}
	} else {
		plaintext = pkt.Data
	}

	switch pkt.Context {
	case packet.ContextNone:
		if l.packetCallback != nil {
			l.packetCallback(plaintext, pkt)
		}
	case packet.ContextRequest:
		return l.handleRequest(plaintext, pkt)
	case packet.ContextResponse:
		return l.handleResponse(plaintext)
	case packet.ContextLinkIdentify:
		return l.HandleIdentification(plaintext)
	case packet.ContextKeepalive:
		if !l.initiator && len(plaintext) == common.ONE && plaintext[common.ZERO] == common.HEX_0xFF {
			keepaliveResp := []byte{0xFE}
			keepalivePkt := &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeData,
				TransportType:   common.ZERO,
				Context:         packet.ContextKeepalive,
				ContextFlag:     packet.FlagUnset,
				Hops:            common.ZERO,
				DestinationType: DEST_TYPE_LINK,
				DestinationHash: l.linkID,
				Data:            keepaliveResp,
				CreateReceipt:   false,
			}
			encrypted, err := l.encrypt(keepaliveResp)
			if err != nil {
				return err
			}
			keepalivePkt.Data = encrypted
			keepalivePkt.Packed = false
			if err := keepalivePkt.Pack(); err != nil {
				return err
			}
			l.lastOutbound = time.Now()
			return l.transport.SendPacket(keepalivePkt)
		}
	case packet.ContextLinkClose:
		return l.handleTeardown(plaintext)
	case packet.ContextLRRTT:
		return l.handleRTTPacket(pkt)
	case packet.ContextResourceAdv:
		return l.handleResourceAdvertisement(pkt)
	case packet.ContextResourceReq:
		return l.handleResourceRequest(pkt)
	case packet.ContextResourceHMU:
		return l.handleResourceHashmapUpdate(pkt)
	case packet.ContextResourceICL:
		return l.handleResourceCancel(pkt)
	case packet.ContextResourceRCL:
		return l.handleResourceReject(pkt)
	case packet.ContextResource:
		return l.handleResourcePart(plaintext, pkt)
	case packet.ContextChannel:
		return l.handleChannelPacket(pkt)
	}

	return nil
}

func (l *Link) GetChannel() *channel.Channel {
	l.channelMutex.Lock()
	defer l.channelMutex.Unlock()

	if l.channel == nil {
		l.channel = channel.NewChannel(l)
	}
	return l.channel
}

func (l *Link) handleChannelPacket(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	l.channelMutex.RLock()
	ch := l.channel
	l.channelMutex.RUnlock()

	if ch != nil {
		return ch.HandleInbound(plaintext)
	}

	return nil
}

func (l *Link) handleResourceAdvertisement(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	adv, err := resource.UnpackResourceAdvertisement(plaintext)
	if err != nil {
		debug.Log(debug.DEBUG_INFO, "Failed to unpack resource advertisement", "error", err)
		return err
	}

	if resource.IsRequestAdvertisement(plaintext) {
		requestID := resource.ReadRequestID(plaintext)
		if l.destination != nil {
			handler := l.destination.GetRequestHandler(requestID)
			if handler != nil {
				response := handler(requestID, nil, requestID, l.linkID, l.remoteIdentity, time.Now())
				if response != nil {
					return l.sendResourceResponse(requestID, response)
				}
			}
		}
		return nil
	}

	if resource.IsResponseAdvertisement(plaintext) {
		requestID := resource.ReadRequestID(plaintext)
		l.requestMutex.Lock()
		for i, req := range l.pendingRequests {
			if string(req.requestID) == string(requestID) {
				req.mutex.Lock()
				req.status = STATUS_ACTIVE
				req.receivedAt = time.Now()
				req.mutex.Unlock()

				if req.responseCb != nil {
					go req.responseCb(req)
				}

				l.pendingRequests = append(l.pendingRequests[:i], l.pendingRequests[i+1:]...)
				break
			}
		}
		l.requestMutex.Unlock()
		return nil
	}

	if l.resourceStrategy == ACCEPT_NONE {
		return nil
	}

	allowed := false
	if l.resourceStrategy == ACCEPT_ALL {
		allowed = true
	} else if l.resourceStrategy == ACCEPT_APP && l.resourceCallback != nil {
		allowed = l.resourceCallback(adv)
	}

	if allowed {
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(adv)
		}
	} else {
		_ = l.rejectResource(adv.Hash) // #nosec G104 - best effort resource rejection
		debug.Log(debug.DEBUG_INFO, "Resource advertisement rejected")
	}

	return nil
}

func (l *Link) rejectResource(resourceHash []byte) error {
	rejectPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextResourceRCL,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
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
	l.lastOutbound = time.Now()
	return l.transport.SendPacket(rejectPkt)
}

func (l *Link) sendResourceResponse(requestID []byte, response any) error {
	resData, ok := response.([]byte)
	if !ok {
		return errors.New("response must be []byte")
	}

	res, err := resource.New(resData, false)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	res.SetRequestID(requestID)
	res.SetIsResponse(true)

	return l.sendResourceAdvertisement(res)
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
		TransportType:   common.ZERO,
		Context:         packet.ContextResourceAdv,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            encrypted,
		CreateReceipt:   false,
	}

	if err := advPkt.Pack(); err != nil {
		return err
	}

	l.lastOutbound = time.Now()
	return l.transport.SendPacket(advPkt)
}

func (l *Link) handleResourceRequest(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
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

	if l.resourceConcludedCallback != nil {
		l.resourceConcludedCallback(plaintext)
	}

	return nil
}

func (l *Link) handleResourceReject(pkt *packet.Packet) error {
	plaintext, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}

	if l.resourceConcludedCallback != nil {
		l.resourceConcludedCallback(plaintext)
	}

	return nil
}

func (l *Link) handleResourcePart(data []byte, pkt *packet.Packet) error {
	if l.resourceStartedCallback != nil {
		l.resourceStartedCallback(data)
	}

	return nil
}

func (l *Link) handleRequest(plaintext []byte, pkt *packet.Packet) error {
	if l.destination == nil {
		return errors.New("no destination for request handling")
	}

	var requestData []any
	if err := msgpack.Unmarshal(plaintext, &requestData); err != nil {
		return fmt.Errorf("failed to unpack request: %w", err)
	}

	if len(requestData) < MIN_REQUEST_DATA_LEN {
		return errors.New("invalid request format")
	}

	requestedAtFloat, ok := requestData[common.ZERO].(float64)
	if !ok {
		requestedAtInt, ok := requestData[common.ZERO].(int64)
		if !ok {
			return fmt.Errorf("invalid requested_at type: %T", requestData[common.ZERO])
		}
		requestedAtFloat = float64(requestedAtInt)
	}
	requestedAt := time.Unix(int64(requestedAtFloat), 0)

	pathHash, ok := requestData[common.ONE].([]byte)
	if !ok {
		return fmt.Errorf("invalid path_hash type: %T", requestData[common.ONE])
	}

	var requestPayload []byte
	if requestData[common.TWO] != nil {
		switch payload := requestData[common.TWO].(type) {
		case []byte:
			requestPayload = payload
		case string:
			requestPayload = []byte(payload)
		default:
			packed, err := msgpack.Marshal(payload)
			if err != nil {
				return fmt.Errorf("failed to pack request_payload: %w", err)
			}
			requestPayload = packed
		}
	}

	requestID := pkt.TruncatedHash()

	debug.Log(debug.DEBUG_INFO, "Handling request", "path_hash", fmt.Sprintf("%x", pathHash), "request_id", fmt.Sprintf("%x", requestID))

	if l.destination != nil {
		handler := l.destination.GetRequestHandler(pathHash)
		if handler != nil {
			response := handler(pathHash, requestPayload, requestID, l.linkID, l.remoteIdentity, requestedAt)
			if response != nil {
				return l.sendResponse(requestID, response)
			}
		} else {
			debug.Log(debug.DEBUG_VERBOSE, "No handler found for path", "path_hash", fmt.Sprintf("%x", pathHash))
		}
	}

	return nil
}

func (l *Link) handleResponse(plaintext []byte) error {
	var responseData []any
	if err := msgpack.Unmarshal(plaintext, &responseData); err != nil {
		return fmt.Errorf("failed to unpack response: %w", err)
	}

	if len(responseData) < MIN_RESPONSE_DATA_LEN {
		return errors.New("invalid response format")
	}

	requestID := responseData[common.ZERO].([]byte)
	responsePayload := responseData[common.ONE].([]byte)

	l.requestMutex.Lock()
	for i, req := range l.pendingRequests {
		if string(req.requestID) == string(requestID) {
			req.mutex.Lock()
			req.status = STATUS_ACTIVE
			req.response = responsePayload
			req.receivedAt = time.Now()
			req.mutex.Unlock()

			if req.responseCb != nil {
				go req.responseCb(req)
			}

			l.pendingRequests = append(l.pendingRequests[:i], l.pendingRequests[i+1:]...)
			break
		}
	}
	l.requestMutex.Unlock()

	return nil
}

func (l *Link) sendResponse(requestID []byte, response any) error {
	responseData := []any{requestID, response}
	packedResponse, err := msgpack.Marshal(responseData)
	if err != nil {
		return fmt.Errorf("failed to pack response: %w", err)
	}

	if len(packedResponse) <= l.mdu {
		encrypted, err := l.encrypt(packedResponse)
		if err != nil {
			return err
		}

		respPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   common.ZERO,
			Context:         packet.ContextResponse,
			ContextFlag:     packet.FlagUnset,
			Hops:            common.ZERO,
			DestinationType: DEST_TYPE_LINK,
			DestinationHash: l.linkID,
			Data:            encrypted,
			CreateReceipt:   false,
		}

		if err := respPkt.Pack(); err != nil {
			return err
		}

		l.lastOutbound = time.Now()
		l.lastDataSent = time.Now()

		if l.networkInterface != nil {
			debug.Log(debug.DEBUG_INFO, "Sending response through interface", "request_id", fmt.Sprintf("%x", requestID), "response_len", len(encrypted), "interface", l.networkInterface.GetName())
			return l.networkInterface.Send(respPkt.Raw, "")
		}

		return l.transport.SendPacket(respPkt)
	}

	return errors.New("response too large, resource transfer not yet implemented")
}

func (l *Link) handleRTTPacket(pkt *packet.Packet) error {
	if !l.initiator {
		measuredRTT := time.Since(l.requestTime).Seconds()
		debug.Log(debug.DEBUG_INFO, "Handling RTT packet (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "has_session_key", l.sessionKey != nil, "status", l.status.Load(), "data_len", len(pkt.Data))
		plaintext, err := l.decrypt(pkt.Data)
		if err != nil {
			debug.Log(debug.DEBUG_ERROR, "Failed to decrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			return err
		}
		debug.Log(debug.DEBUG_INFO, "RTT packet decrypted successfully", "plaintext_len", len(plaintext), "link_id", fmt.Sprintf("%x", l.linkID))

		var rtt float64
		if len(plaintext) >= common.EIGHT {
			rtt = float64(binary.BigEndian.Uint64(plaintext[:common.EIGHT])) / common.FLOAT_1E9
		}

		l.rtt = maxFloat(measuredRTT, rtt)
		l.status.Store(int32(STATUS_ACTIVE))
		l.establishedAt = time.Now()

		if l.transport != nil {
			l.transport.RegisterLink(l.linkID, l)
			if l.networkInterface != nil {
				l.transport.UpdatePath(l.linkID, nil, l.networkInterface.GetName(), 0)
			}
		}

		if l.rtt > 0 {
			l.updateKeepalive()
		}

		// Ensure transport has a path for the link ID
		if l.transport != nil && l.networkInterface != nil {
			l.transport.UpdatePath(l.linkID, nil, l.networkInterface.GetName(), 0)
		}

		if l.establishedCallback != nil {
			go l.establishedCallback(l)
		}

		establishmentElapsed := time.Since(l.requestTime).Seconds()
		debug.Log(debug.DEBUG_INFO, "Link established (responder) after RTT", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", l.rtt), "total_elapsed", fmt.Sprintf("%.3fs", establishmentElapsed))
	}
	return nil
}

func (l *Link) updateKeepalive() {
	if l.rtt <= 0 {
		return
	}

	keepaliveMaxRTT := common.FLOAT_1_75
	keepaliveMax := float64(KEEPALIVE)
	keepaliveMin := common.FLOAT_5_0

	calculatedKeepalive := l.rtt * (keepaliveMax / keepaliveMaxRTT)
	if calculatedKeepalive > keepaliveMax {
		calculatedKeepalive = keepaliveMax
	}
	if calculatedKeepalive < keepaliveMin {
		calculatedKeepalive = keepaliveMin
	}

	l.keepalive = time.Duration(calculatedKeepalive * float64(time.Second))
	l.staleTime = time.Duration(float64(l.keepalive) * float64(common.TWO))
}

func (l *Link) handleLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	if l.initiator {
		return l.ValidateLinkProof(pkt, networkIface)
	}
	return nil
}

func (l *Link) handleTeardown(plaintext []byte) error {
	if len(plaintext) == len(l.linkID) && string(plaintext) == string(l.linkID) {
		l.status.Store(int32(STATUS_CLOSED))
		if l.initiator {
			l.teardownReason = STATUS_FAILED
		} else {
			l.teardownReason = STATUS_FAILED
		}
		if l.closedCallback != nil {
			l.closedCallback(l)
		}
	}
	return nil
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (l *Link) encrypt(data []byte) ([]byte, error) {
	if l.sessionKey == nil || l.hmacKey == nil {
		return nil, errors.New("no session keys available")
	}

	block, err := aes.NewCipher(l.sessionKey)
	if err != nil {
		return nil, err
	}

	// Generate IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	// Add PKCS7 padding
	padding := aes.BlockSize - len(data)%aes.BlockSize
	padtext := make([]byte, len(data)+padding)
	copy(padtext, data)
	for i := len(data); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	// Encrypt
	mode := cipher.NewCBCEncrypter(block, iv) // #nosec G407
	ciphertext := make([]byte, len(padtext))
	mode.CryptBlocks(ciphertext, padtext)

	// Combine IV and ciphertext for HMAC
	signedParts := make([]byte, len(iv)+len(ciphertext))
	copy(signedParts, iv)
	copy(signedParts[len(iv):], ciphertext)

	// Calculate HMAC
	h := hmac.New(sha256.New, l.hmacKey)
	h.Write(signedParts)
	mac := h.Sum(nil)

	// Result: [IV] [Ciphertext] [HMAC]
	result := make([]byte, len(signedParts)+len(mac))
	copy(result, signedParts)
	copy(result[len(signedParts):], mac)
	return result, nil
}

func (l *Link) decrypt(data []byte) ([]byte, error) {
	if l.sessionKey == nil || l.hmacKey == nil {
		debug.Log(debug.DEBUG_ERROR, "Decrypt failed: no session keys", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil, errors.New("no session keys available")
	}

	// Minimum length: IV(16) + at least one block(16) + HMAC(32) = 64 bytes
	if len(data) < aes.BlockSize+aes.BlockSize+common.SIZE_32 {
		debug.Log(debug.DEBUG_ERROR, "Decrypt failed: data too short", "length", len(data))
		return nil, errors.New("data too short")
	}

	// Split into [IV + Ciphertext] and [HMAC]
	signedParts := data[:len(data)-common.SIZE_32]
	receivedMac := data[len(data)-common.SIZE_32:]

	// Verify HMAC
	h := hmac.New(sha256.New, l.hmacKey)
	h.Write(signedParts)
	expectedMac := h.Sum(nil)
	if !hmac.Equal(receivedMac, expectedMac) {
		debug.Log(debug.DEBUG_ERROR, "Decrypt failed: HMAC mismatch", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil, errors.New("HMAC verification failed")
	}

	// Extract IV and ciphertext
	iv := signedParts[:aes.BlockSize]
	ciphertext := signedParts[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		debug.Log(debug.DEBUG_ERROR, "Decrypt failed: ciphertext not multiple of block size", "length", len(ciphertext))
		return nil, errors.New("ciphertext is not a multiple of block size")
	}

	block, err := aes.NewCipher(l.sessionKey)
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return nil, errors.New("invalid padding: plaintext empty")
	}

	padding := int(plaintext[len(plaintext)-1])
	if padding > aes.BlockSize || padding == 0 {
		debug.Log(debug.DEBUG_ERROR, "Decrypt failed: invalid padding value", "padding", padding)
		return nil, errors.New("invalid padding")
	}

	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			debug.Log(debug.DEBUG_ERROR, "Decrypt failed: padding byte mismatch", "expected", padding, "got", plaintext[i])
			return nil, errors.New("invalid padding")
		}
	}

	return plaintext[:len(plaintext)-padding], nil
}

func (l *Link) GetRTT() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.rtt
}

func (l *Link) RTT() float64 {
	return l.GetRTT()
}

func (l *Link) SetRTT(rtt float64) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.rtt = rtt
}

func (l *Link) GetStatus() byte {
	switch l.status.Load() {
	case int32(STATUS_PENDING):
		return STATUS_PENDING
	case int32(STATUS_HANDSHAKE):
		return STATUS_HANDSHAKE
	case int32(STATUS_ACTIVE):
		return STATUS_ACTIVE
	case int32(STATUS_STALE):
		return STATUS_STALE
	case int32(STATUS_CLOSED):
		return STATUS_CLOSED
	case int32(STATUS_FAILED):
		return STATUS_FAILED
	default:
		return STATUS_FAILED
	}
}

func (l *Link) Send(data []byte) any {
	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextChannel,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            data,
		CreateReceipt:   false,
	}

	encrypted, err := l.encrypt(data)
	if err != nil {
		return nil
	}
	pkt.Data = encrypted

	if err := pkt.Pack(); err != nil {
		return nil
	}

	l.lastOutbound = time.Now()
	if err := l.transport.SendPacket(pkt); err != nil {
		return nil
	}

	return pkt
}

func (l *Link) SetPacketTimeout(pkt any, callback func(any), timeout time.Duration) {
	if packetObj, ok := pkt.(*packet.Packet); ok {
		go func() {
			time.Sleep(timeout)
			if callback != nil {
				callback(packetObj)
			}
		}()
	}
}

func (l *Link) SetPacketDelivered(pkt any, callback func(any)) {
	if callback != nil {
		go callback(pkt)
	}
}

func (l *Link) Resend(pkt any) error {
	packetObj, ok := pkt.(*packet.Packet)
	if !ok {
		return errors.New("invalid packet type")
	}

	return l.transport.SendPacket(packetObj)
}

func (l *Link) GetLinkID() []byte {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.linkID
}

func (l *Link) IsActive() bool {
	return l.GetStatus() == STATUS_ACTIVE
}

func (l *Link) SendResource(res *resource.Resource) error {
	l.mutex.Lock()
	if l.status.Load() != int32(STATUS_ACTIVE) {
		l.teardownReason = STATUS_FAILED
		l.mutex.Unlock()
		return errors.New("link not active")
	}
	sdu := l.mdu
	l.mutex.Unlock()

	if err := res.PrepareOutboundForLink(l.encrypt, sdu); err != nil {
		return err
	}

	l.mutex.Lock()
	res.Activate()
	l.mutex.Unlock()

	if err := l.sendResourceAdvertisement(res); err != nil {
		l.mutex.Lock()
		l.teardownReason = STATUS_FAILED
		l.mutex.Unlock()
		return fmt.Errorf("resource advertisement: %w", err)
	}

	// Do not hold l.mutex while sending: SendPacketWithContext locks the same mutex.
	buffer := make([]byte, sdu)
	for {
		n, err := res.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			l.mutex.Lock()
			l.teardownReason = STATUS_FAILED
			l.mutex.Unlock()
			return fmt.Errorf("error reading resource: %v", err)
		}

		if err := l.SendPacketWithContext(buffer[:n], packet.ContextResource); err != nil {
			l.mutex.Lock()
			l.teardownReason = STATUS_FAILED
			l.mutex.Unlock()
			return fmt.Errorf("error sending resource packet: %v", err)
		}
	}

	if err := l.SendPacketWithContext(nil, packet.ContextResource); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to send resource completion packet", "error", err)
	}

	return nil
}

func (l *Link) maintainLink() {
	ticker := time.NewTicker(time.Second * KEEPALIVE)
	defer ticker.Stop()

	for range ticker.C {
		if l.status.Load() != int32(STATUS_ACTIVE) {
			return
		}

		inactiveTime := l.InactiveFor()
		if inactiveTime > float64(STALE_TIME) {
			l.mutex.Lock()
			l.teardownReason = STATUS_FAILED
			l.mutex.Unlock()
			l.Teardown()
			return
		}

		noDataTime := l.NoDataFor()
		if noDataTime > float64(KEEPALIVE) {
			l.mutex.Lock()
			err := l.SendPacket([]byte{})
			if err != nil {
				l.teardownReason = STATUS_FAILED
				l.mutex.Unlock()
				l.Teardown()
				return
			}
			l.mutex.Unlock()
		}
	}
}

func (l *Link) Start() {
	go l.maintainLink()
}

func (l *Link) SetProofStrategy(strategy byte) error {
	if strategy != PROVE_NONE && strategy != PROVE_ALL && strategy != PROVE_APP {
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

func (l *Link) HandleProofRequest(packet *packet.Packet) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	switch l.proofStrategy {
	case PROVE_NONE:
		return false
	case PROVE_ALL:
		return true
	case PROVE_APP:
		if l.proofCallback != nil {
			return l.proofCallback(packet)
		}
		return false
	default:
		return false
	}
}

func (l *Link) decodePacket(data []byte) {
	if len(data) < 1 {
		debug.Log(debug.DEBUG_ALL, "Invalid packet: zero length")
		return
	}

	packetType := data[0]
	debug.Log(debug.DEBUG_ALL, "Packet Analysis", "size", len(data), "type", fmt.Sprintf(common.STR_FMT_HEX, packetType))

	switch packetType {
	case packet.PacketTypeData:
		debug.Log(debug.DEBUG_ALL, "Type Description: Data Packet", "payload_size", len(data)-common.ONE)

	case packet.PacketTypeLinkReq:
		debug.Log(debug.DEBUG_ALL, "Type Description: Link Management", common.STR_LINK_ID, fmt.Sprintf("%x", data[common.ONE:common.SIZE_32+common.ONE]))

	case packet.PacketTypeAnnounce:
		debug.Log(debug.DEBUG_ALL, "Received announce packet", common.STR_BYTES, len(data))
		if len(data) < packet.MinAnnounceSize {
			debug.Log(debug.DEBUG_INFO, "Announce packet too short", "bytes", len(data))
			return
		}

		destHash := data[common.TWO : common.SIZE_16+common.TWO]
		encKey := data[common.SIZE_16+common.TWO : common.SIZE_32+common.SIZE_16+common.TWO]
		signKey := data[common.SIZE_32+common.SIZE_16+common.TWO : common.SIZE_32*common.TWO+common.SIZE_16+common.TWO]
		nameHash := data[common.SIZE_32*common.TWO+common.SIZE_16+common.TWO : common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT+common.TWO]
		randomHash := data[common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT+common.TWO : common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT*common.TWO+common.TWO]
		signature := data[common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT*common.TWO+common.TWO : common.SIZE_64+common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT*common.TWO+common.TWO]
		appData := data[common.SIZE_64+common.SIZE_32*common.TWO+common.SIZE_16+common.TWO+common.EIGHT*common.TWO+common.TWO:]

		pubKey := append(encKey, signKey...)

		validationData := make([]byte, common.ZERO, common.SIZE_32*common.FIVE+common.FOUR)
		validationData = append(validationData, destHash...)
		validationData = append(validationData, encKey...)
		validationData = append(validationData, signKey...)
		validationData = append(validationData, nameHash...)
		validationData = append(validationData, randomHash...)

		if identity.ValidateAnnounce(validationData, destHash, pubKey, signature, appData) {
			debug.Log(debug.DEBUG_VERBOSE, "Valid announce from", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
			if err := l.transport.HandleAnnounce(destHash, l.networkInterface); err != nil {
				debug.Log(debug.DEBUG_INFO, "Failed to handle announce", "error", err)
			}
		} else {
			debug.Log(debug.DEBUG_INFO, "Invalid announce signature from", "public_key", fmt.Sprintf("%x", pubKey[:common.EIGHT]))
		}

	case packet.PacketTypeProof:
		debug.Log(debug.DEBUG_ALL, "Type Description: RNS Discovery")
		if len(data) > common.SIZE_16+common.ONE {
			searchHash := data[common.ONE : common.SIZE_16+common.ONE]
			debug.Log(debug.DEBUG_ALL, "Searching for Hash", "search_hash", fmt.Sprintf("%x", searchHash))

			if id, err := resolver.ResolveIdentity(hex.EncodeToString(searchHash)); err == nil {
				debug.Log(debug.DEBUG_ALL, "Found matching identity", "identity_hash", id.GetHexHash())
			}
		}

	default:
		debug.Log(debug.DEBUG_ALL, "Type Description: Unknown", "type", fmt.Sprintf("0x%02x", packetType), "raw_hex", fmt.Sprintf("%x", data))
	}
}

func (l *Link) startWatchdog() {
	if l.watchdogActive {
		return
	}

	l.watchdogActive = true
	go l.watchdog()
}

func (l *Link) watchdog() {
	for l.GetStatus() != STATUS_CLOSED {
		l.mutex.Lock()
		if l.watchdogLock {
			rttWait := common.FLOAT_0_025
			if l.rtt > common.FLOAT_ZERO {
				rttWait = l.rtt
			}
			if rttWait < common.FLOAT_0_025 {
				rttWait = common.FLOAT_0_025
			}
			l.mutex.Unlock()
			time.Sleep(time.Duration(rttWait * float64(time.Second)))
			continue
		}

		var sleepTime = WATCHDOG_INTERVAL

		if l.status.Load() == int32(STATUS_PENDING) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				debug.Log(debug.DEBUG_INFO, "Link establishment timed out", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load())
				l.status.Store(int32(STATUS_CLOSED))
				l.teardownReason = STATUS_FAILED
				if l.closedCallback != nil {
					l.closedCallback(l)
				}
				sleepTime = common.FLOAT_0_001
			}
		} else if l.status.Load() == int32(STATUS_HANDSHAKE) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				elapsed := time.Since(l.requestTime).Seconds()
				if l.initiator {
					debug.Log(debug.DEBUG_INFO, "Timeout waiting for link request proof", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				} else {
					debug.Log(debug.DEBUG_INFO, "Timeout waiting for RTT packet from link initiator", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				}
				l.status.Store(int32(STATUS_CLOSED))
				l.teardownReason = STATUS_FAILED
				if l.closedCallback != nil {
					l.closedCallback(l)
				}
				sleepTime = 0.001
			}
		} else if l.status.Load() == int32(STATUS_ACTIVE) {
			activatedAt := l.establishedAt
			if activatedAt.IsZero() {
				activatedAt = time.Time{}
			}
			lastActivity := l.lastInbound
			if l.lastOutbound.After(lastActivity) {
				lastActivity = l.lastOutbound
			}
			if l.lastDataSent.After(lastActivity) {
				lastActivity = l.lastDataSent
			}
			if lastActivity.Before(activatedAt) {
				lastActivity = activatedAt
			}
			now := time.Now()

			if now.After(lastActivity.Add(l.keepalive)) {
				if l.initiator {
					lastKeepalive := l.lastOutbound
					if now.After(lastKeepalive.Add(l.keepalive)) {
						_ = l.sendKeepalive() // #nosec G104 - best effort keepalive
					}
				}

				if now.After(lastActivity.Add(l.staleTime)) {
					sleepTime = l.rtt*KEEPALIVE_TIMEOUT_FACTOR + STALE_GRACE
					l.status.Store(int32(STATUS_STALE))
				} else {
					sleepTime = float64(l.keepalive) / float64(time.Second)
				}
			} else {
				nextKeepalive := lastActivity.Add(l.keepalive)
				sleepTime = time.Until(nextKeepalive).Seconds()
			}
		} else if l.status.Load() == int32(STATUS_STALE) {
			sleepTime = common.FLOAT_0_001
			debug.Log(debug.DEBUG_INFO, "Link marked stale, closing", "link_id", fmt.Sprintf("%x", l.linkID))
			_ = l.sendTeardownPacket() // #nosec G104 - best effort teardown
			l.status.Store(int32(STATUS_CLOSED))
			l.teardownReason = STATUS_FAILED
			if l.closedCallback != nil {
				l.closedCallback(l)
			}
			sleepTime = common.FLOAT_0_001
		}

		if sleepTime <= common.FLOAT_ZERO {
			sleepTime = common.FLOAT_0_1
		}
		if sleepTime > common.FLOAT_5_0 {
			sleepTime = common.FLOAT_5_0
		}

		l.mutex.Unlock()
		time.Sleep(time.Duration(sleepTime * float64(time.Second)))
	}
	l.watchdogActive = false
}

func (l *Link) sendKeepalive() error {
	keepaliveData := []byte{0xFF}
	keepalivePkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextKeepalive,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            keepaliveData,
		CreateReceipt:   false,
	}
	encrypted, err := l.encrypt(keepaliveData)
	if err != nil {
		return err
	}
	keepalivePkt.Data = encrypted
	if err := keepalivePkt.Pack(); err != nil {
		return err
	}
	l.lastOutbound = time.Now()
	return l.transport.SendPacket(keepalivePkt)
}

func (l *Link) sendTeardownPacket() error {
	teardownPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextLinkClose,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            l.linkID,
		CreateReceipt:   false,
	}
	encrypted, err := l.encrypt(l.linkID)
	if err != nil {
		return err
	}
	teardownPkt.Data = encrypted
	if err := teardownPkt.Pack(); err != nil {
		return err
	}
	l.lastOutbound = time.Now()
	return l.transport.SendPacket(teardownPkt)
}

func (l *Link) Validate(signature, message []byte) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if l.remoteIdentity == nil {
		return false
	}

	return l.remoteIdentity.Verify(message, signature)
}

func (l *Link) generateEphemeralKeys() error {
	priv, pub, err := cryptography.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate X25519 keypair: %w", err)
	}
	l.prv = priv
	l.pub = pub

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 keypair: %w", err)
	}
	l.sigPriv = privKey
	l.sigPub = pubKey

	return nil
}

func signallingBytes(mtu int, mode byte) []byte {
	bytes := make([]byte, LINK_MTU_SIZE)
	bytes[common.ZERO] = byte((mtu >> common.SIZE_16) & common.HEX_0xFF)
	bytes[common.ONE] = byte((mtu >> common.EIGHT) & common.HEX_0xFF)
	bytes[common.TWO] = byte(mtu & common.HEX_0xFF)
	bytes[common.ZERO] |= (mode << common.FIVE)
	return bytes
}

func (l *Link) SendLinkRequest() error {
	if err := l.generateEphemeralKeys(); err != nil {
		return err
	}

	l.mode = MODE_DEFAULT
	l.mtu = common.DEFAULT_MTU / common.THREE
	l.updateMDU()

	signalling := signallingBytes(l.mtu, l.mode)
	requestData := make([]byte, 0, ECPUBSIZE+LINK_MTU_SIZE)
	requestData = append(requestData, l.pub...)
	requestData = append(requestData, l.sigPub...)
	requestData = append(requestData, signalling...)

	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeLinkReq,
		TransportType:   common.ZERO,
		Context:         packet.ContextNone,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
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
	l.status.Store(int32(STATUS_PENDING))

	sendStartTime := time.Now()
	if err := l.transport.SendPacket(pkt); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to send link request", "error", err, "elapsed", time.Since(sendStartTime).Seconds())
		return fmt.Errorf("failed to send link request: %w", err)
	}

	debug.Log(debug.DEBUG_INFO, "Link request sent", "link_id", fmt.Sprintf("%x", l.linkID), "send_elapsed", time.Since(sendStartTime).Seconds(), "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	return nil
}

func linkIDFromPacket(pkt *packet.Packet) []byte {
	hashablePart := []byte{pkt.Raw[common.ZERO] & 0b00001111}

	if pkt.HeaderType == packet.HeaderType2 {
		dstLen := common.SIZE_16
		startIndex := dstLen + common.TWO
		if len(pkt.Raw) > startIndex {
			hashablePart = append(hashablePart, pkt.Raw[startIndex:]...)
		}
	} else {
		if len(pkt.Raw) > common.TWO {
			hashablePart = append(hashablePart, pkt.Raw[common.TWO:]...)
		}
	}

	if len(pkt.Data) > ECPUBSIZE {
		diff := len(pkt.Data) - ECPUBSIZE
		if len(hashablePart) >= diff {
			hashablePart = hashablePart[:len(hashablePart)-diff]
		}
	}

	return identity.TruncatedHash(hashablePart)
}

func (l *Link) HandleLinkRequest(pkt *packet.Packet, ownerIdentity *identity.Identity) error {
	startTime := time.Now()
	debug.Log(debug.DEBUG_INFO, "Handling incoming link request", "data_len", len(pkt.Data), "has_interface", l.networkInterface != nil, "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	if len(pkt.Data) < ECPUBSIZE {
		return errors.New("link request data too short")
	}

	peerPub := pkt.Data[common.ZERO:KEYSIZE]
	peerSigPub := pkt.Data[KEYSIZE:ECPUBSIZE]

	l.peerPub = peerPub
	l.peerSigPub = peerSigPub
	l.linkID = linkIDFromPacket(pkt)
	l.initiator = false

	myPubStr := "not_generated_yet"
	if len(l.pub) >= 8 {
		myPubStr = fmt.Sprintf("%x", l.pub[:8])
	}
	debug.Log(debug.DEBUG_INFO, "Link request processed (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "my_pub", myPubStr, "elapsed", time.Since(startTime).Seconds())

	if len(pkt.Data) >= ECPUBSIZE+LINK_MTU_SIZE {
		mtuBytes := pkt.Data[ECPUBSIZE : ECPUBSIZE+LINK_MTU_SIZE]
		l.mtu = (int(mtuBytes[0]&0x1F) << 16) | (int(mtuBytes[1]) << 8) | int(mtuBytes[2])
		l.mode = (mtuBytes[0] & MODE_BYTEMASK) >> 5
		debug.Log(debug.DEBUG_VERBOSE, "Link request includes MTU", "mtu", l.mtu, "mode", l.mode)
	} else {
		l.mtu = common.DEFAULT_MTU / common.THREE
		l.mode = MODE_DEFAULT
	}

	if err := l.generateEphemeralKeys(); err != nil {
		return err
	}

	debug.Log(debug.DEBUG_INFO, "Ephemeral keys generated (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "my_pub", fmt.Sprintf("%x", l.pub[:8]), "peer_pub", fmt.Sprintf("%x", l.peerPub[:8]))

	if err := l.performHandshake(); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	proofStartTime := time.Now()
	if err := l.sendLinkProof(ownerIdentity); err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to send link proof", "error", err, "elapsed", time.Since(proofStartTime).Seconds())
		return fmt.Errorf("failed to send link proof: %w", err)
	}

	l.status.Store(int32(STATUS_HANDSHAKE))
	l.lastInbound = time.Now()
	l.requestTime = time.Now()

	if l.transport != nil {
		l.transport.RegisterLink(l.linkID, l)
		if l.networkInterface != nil {
			l.transport.UpdatePath(l.linkID, nil, l.networkInterface.GetName(), 0)
		}
	}

	debug.Log(debug.DEBUG_INFO, "Link proof sent (responder), waiting for RTT", "link_id", fmt.Sprintf("%x", l.linkID), "proof_send_elapsed", time.Since(proofStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())

	return nil
}

func (l *Link) updateMDU() {
	headerMaxSize := common.SIZE_64
	ifacMinSize := common.FOUR
	tokenOverhead := common.TOKEN_OVERHEAD
	aesBlockSize := common.SIZE_16

	l.mdu = int(float64(l.mtu-headerMaxSize-ifacMinSize-tokenOverhead)/float64(aesBlockSize))*aesBlockSize - common.ONE
	if l.mdu < common.ZERO {
		l.mdu = common.DEFAULT_MTU / common.FIFTEEN
	}
}

func (l *Link) performHandshake() error {
	if len(l.peerPub) != KEYSIZE {
		return errors.New("invalid peer public key length")
	}

	sharedSecret, err := cryptography.DeriveSharedSecret(l.prv, l.peerPub)
	if err != nil {
		return fmt.Errorf("ECDH failed: %w", err)
	}
	l.sharedKey = sharedSecret

	var derivedKeyLength int
	if l.mode == MODE_AES128_CBC {
		derivedKeyLength = 32
	} else if l.mode == MODE_AES256_CBC {
		derivedKeyLength = 64
	} else {
		return fmt.Errorf("invalid link mode: %d", l.mode)
	}

	derivedKey, err := cryptography.DeriveKey(l.sharedKey, l.linkID, nil, derivedKeyLength)
	if err != nil {
		return fmt.Errorf("HKDF failed: %w", err)
	}
	l.derivedKey = derivedKey

	if len(derivedKey) >= common.SIZE_64 {
		l.hmacKey = derivedKey[0:common.SIZE_32]
		l.sessionKey = derivedKey[common.SIZE_32:common.SIZE_64]
		debug.Log(debug.DEBUG_INFO, "Session keys derived", "link_id", fmt.Sprintf("%x", l.linkID), "mode", l.mode, "initiator", l.initiator, "hmac_key", fmt.Sprintf("%x", l.hmacKey[:8]), "session_key", fmt.Sprintf("%x", l.sessionKey[:8]))
	} else if len(derivedKey) >= common.SIZE_32 {
		l.hmacKey = derivedKey[0:common.SIZE_16]
		l.sessionKey = derivedKey[common.SIZE_16:common.SIZE_32]
	}

	l.status.Store(int32(STATUS_HANDSHAKE))
	debug.Log(debug.DEBUG_VERBOSE, "Handshake completed", "key_material_bytes", len(derivedKey), "shared_key", fmt.Sprintf("%x", l.sharedKey[:8]), "link_id", fmt.Sprintf("%x", l.linkID))
	return nil
}

func (l *Link) sendLinkProof(ownerIdentity *identity.Identity) error {
	debug.Log(debug.DEBUG_ERROR, "Generating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "initiator", l.initiator, "has_interface", l.networkInterface != nil)

	proofPkt, err := l.GenerateLinkProof(ownerIdentity)
	if err != nil {
		return err
	}

	debug.Log(debug.DEBUG_ERROR, "Link proof packet created", "dest_hash", fmt.Sprintf("%x", proofPkt.DestinationHash), "packet_type", fmt.Sprintf(common.STR_FMT_HEX, proofPkt.PacketType))

	// For responder links (not initiator), send proof directly through the receiving interface
	if !l.initiator && l.networkInterface != nil {
		if err := proofPkt.Pack(); err != nil {
			return fmt.Errorf("failed to pack proof packet: %w", err)
		}

		debug.Log(debug.DEBUG_ERROR, "Sending proof through interface", "raw_len", len(proofPkt.Raw), "interface", l.networkInterface.GetName())

		if err := l.networkInterface.Send(proofPkt.Raw, ""); err != nil {
			return fmt.Errorf("failed to send link proof through interface: %w", err)
		}
		debug.Log(debug.DEBUG_ERROR, "Link proof sent through interface", "link_id", fmt.Sprintf("%x", l.linkID), "interface", l.networkInterface.GetName())
		return nil
	}

	// For initiator links, use transport (path lookup)
	if l.transport != nil {
		if err := l.transport.SendPacket(proofPkt); err != nil {
			return fmt.Errorf("failed to send link proof: %w", err)
		}
		debug.Log(debug.DEBUG_INFO, "Link proof sent", "link_id", fmt.Sprintf("%x", l.linkID))
	}

	return nil
}

func (l *Link) GenerateLinkProof(ownerIdentity *identity.Identity) (*packet.Packet, error) {
	signalling := signallingBytes(l.mtu, l.mode)

	ownerSigPub := ownerIdentity.GetPublicKey()[KEYSIZE:ECPUBSIZE]

	signedData := make([]byte, 0, len(l.linkID)+KEYSIZE+len(ownerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, l.pub...)
	signedData = append(signedData, ownerSigPub...)
	signedData = append(signedData, signalling...)

	signature := ownerIdentity.Sign(signedData)

	proofData := make([]byte, 0, len(signature)+KEYSIZE+len(signalling))
	proofData = append(proofData, signature...)
	proofData = append(proofData, l.pub...)
	proofData = append(proofData, signalling...)

	proofPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeProof,
		TransportType:   common.ZERO,
		Context:         packet.ContextLRProof,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
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
	startTime := time.Now()
	debug.Log(debug.DEBUG_INFO, "Validating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load(), "initiator", l.initiator, "has_interface", networkIface != nil, "proof_data_len", len(pkt.Data))
	st := l.status.Load()
	if st != int32(STATUS_PENDING) && st != int32(STATUS_HANDSHAKE) {
		return fmt.Errorf("invalid link status for proof validation: %d", l.status.Load())
	}

	if len(pkt.Data) < identity.SIGLENGTH/8+KEYSIZE {
		return errors.New("link proof data too short")
	}

	signature := pkt.Data[common.ZERO : identity.SIGLENGTH/common.EIGHT]
	peerPub := pkt.Data[identity.SIGLENGTH/common.EIGHT : identity.SIGLENGTH/common.EIGHT+KEYSIZE]

	signalling := []byte{common.ZERO, common.ZERO, common.ZERO}
	if len(pkt.Data) >= identity.SIGLENGTH/8+KEYSIZE+LINK_MTU_SIZE {
		signalling = pkt.Data[identity.SIGLENGTH/8+KEYSIZE : identity.SIGLENGTH/8+KEYSIZE+LINK_MTU_SIZE]
		mtu := (int(signalling[common.ZERO]&0x1F) << common.SIZE_16) | (int(signalling[common.ONE]) << common.EIGHT) | int(signalling[common.TWO])
		mode := (signalling[common.ZERO] & MODE_BYTEMASK) >> common.FIVE
		l.mtu = mtu
		l.mode = mode
		debug.Log(debug.DEBUG_VERBOSE, "Link proof includes MTU", "mtu", mtu, "mode", mode)
	}

	l.peerPub = peerPub
	if l.destination != nil && l.destination.GetIdentity() != nil {
		destIdent := l.destination.GetIdentity()
		pubKey := destIdent.GetPublicKey()
		if len(pubKey) >= ECPUBSIZE {
			l.peerSigPub = pubKey[KEYSIZE:ECPUBSIZE]
		}
	}

	signedData := make([]byte, 0, len(l.linkID)+KEYSIZE+len(l.peerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, peerPub...)
	signedData = append(signedData, l.peerSigPub...)
	signedData = append(signedData, signalling...)

	first32Len := min(len(signedData), 32)
	debug.Log(debug.DEBUG_INFO, "Constructed signed data for validation", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "peer_sig_pub", fmt.Sprintf("%x", l.peerSigPub[:8]), "signalling", fmt.Sprintf("%x", signalling), "signed_data_len", len(signedData), "signed_data_first32", fmt.Sprintf("%x", signedData[:first32Len]))

	if l.destination == nil || l.destination.GetIdentity() == nil {
		return errors.New("no destination identity for proof validation")
	}

	if !l.destination.GetIdentity().Verify(signedData, signature) {
		debug.Log(debug.DEBUG_ERROR, "Link proof signature validation failed", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "signature", fmt.Sprintf("%x", signature[:8]), "signed_data", fmt.Sprintf("%x", signedData))
		return errors.New("link proof signature validation failed")
	}
	debug.Log(debug.DEBUG_INFO, "Link proof signature validated successfully", "link_id", fmt.Sprintf("%x", l.linkID[:8]))

	if err := l.performHandshake(); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	l.rtt = time.Since(l.requestTime).Seconds()
	l.status.Store(int32(STATUS_ACTIVE))
	l.establishedAt = time.Now()

	if l.rtt > 0 {
		l.updateKeepalive()
	}

	rttData := make([]byte, 8)
	binary.BigEndian.PutUint64(rttData, uint64(l.rtt*common.FLOAT_1E9))
	rttPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   common.ZERO,
		Context:         packet.ContextLRRTT,
		ContextFlag:     packet.FlagUnset,
		Hops:            common.ZERO,
		DestinationType: DEST_TYPE_LINK,
		DestinationHash: l.linkID,
		Data:            rttData,
		CreateReceipt:   false,
	}
	encrypted, err := l.encrypt(rttData)
	if err != nil {
		debug.Log(debug.DEBUG_ERROR, "Failed to encrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
	} else {
		rttPkt.Data = encrypted
		if err := rttPkt.Pack(); err != nil {
			debug.Log(debug.DEBUG_ERROR, "Failed to pack RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
		} else {
			if networkIface != nil {
				debug.Log(debug.DEBUG_INFO, "Sending RTT packet through interface", "interface", networkIface.GetName(), "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", l.rtt), "packet_size", len(rttPkt.Raw))
				if err := networkIface.Send(rttPkt.Raw, ""); err != nil {
					debug.Log(debug.DEBUG_ERROR, "Failed to send RTT packet through interface", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
				} else {
					l.lastOutbound = time.Now()
					debug.Log(debug.DEBUG_INFO, "RTT packet sent successfully", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", l.rtt))
				}
			} else {
				debug.Log(debug.DEBUG_INFO, "No network interface for RTT packet, using transport", "link_id", fmt.Sprintf("%x", l.linkID))
				if err := l.transport.SendPacket(rttPkt); err != nil {
					debug.Log(debug.DEBUG_ERROR, "Failed to send RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
				} else {
					l.lastOutbound = time.Now()
					debug.Log(debug.DEBUG_INFO, "RTT packet sent via transport", "link_id", fmt.Sprintf("%x", l.linkID))
				}
			}
		}
	}

	if l.transport != nil {
		l.transport.RegisterLink(l.linkID, l)
		if l.networkInterface != nil {
			l.transport.UpdatePath(l.linkID, nil, l.networkInterface.GetName(), 0)
		}
	}

	establishmentElapsed := time.Since(l.requestTime).Seconds()
	debug.Log(debug.DEBUG_INFO, "Link established (initiator)", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", l.rtt), "total_elapsed", fmt.Sprintf("%.3fs", establishmentElapsed), "validation_elapsed", fmt.Sprintf("%.3fs", time.Since(startTime).Seconds()))

	if l.establishedCallback != nil {
		go l.establishedCallback(l)
	}

	return nil
}
