// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package link

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/channel"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/cryptography"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/pathfinder"
	"quad4/reticulum-go/pkg/resource"
	"quad4/reticulum-go/pkg/transport"
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
	mutex              sync.RWMutex
	destination        *destination.Destination
	status             atomic.Int32
	networkInterface   common.NetworkInterface
	establishedAt      time.Time
	lastInboundNs      atomic.Int64
	lastOutboundNs     atomic.Int64
	lastDataReceivedNs atomic.Int64
	lastDataSentNs     atomic.Int64
	pathFinder         *pathfinder.PathFinder

	remoteIdentity *identity.Identity
	sessionKey     []byte
	linkID         []byte

	rtt               float64
	establishmentRate float64

	establishedCallback func(*Link)
	closedCallback      func(*Link)
	packetCallback      func([]byte, *packet.Packet)
	packetCbMu          sync.RWMutex
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

	incomingMu              sync.Mutex
	incomingRx              *incomingResourceAsm
	incomingResourceRequest *RequestReceipt

	outgoingMu              sync.Mutex
	resourceSendMu          sync.Mutex
	outgoingRes             *resource.Resource
	outgoingReceiverMinPart int
	outgoingResCompleteChan chan struct{}
	outgoingDispatchMu      sync.Mutex

	pendingPlainMu   sync.Mutex
	pendingPlainData []byte
}

func NewLink(dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface, establishedCallback func(*Link), closedCallback func(*Link)) *Link {
	return &Link{
		destination:         dest,
		transport:           transport,
		networkInterface:    networkIface,
		establishedCallback: establishedCallback,
		closedCallback:      closedCallback,
		establishedAt:       time.Time{},
		pathFinder:          pathfinder.NewPathFinder(),

		watchdogLock:         false,
		watchdogActive:       false,
		establishmentTimeout: time.Duration(EstablishmentTimeoutPerHop * float64(time.Second)),
		keepalive:            time.Duration(Keepalive * float64(time.Second)),
		staleTime:            time.Duration(StaleTime * float64(time.Second)),
		initiator:            false,
		pendingRequests:      make([]*RequestReceipt, 0),
	}
}

func HandleIncomingLinkRequest(pkt *packet.Packet, dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface) (*Link, error) {
	startTime := time.Now()
	debug.Log(debug.DebugInfo, "Creating link for incoming request", "dest_hash", fmt.Sprintf("%x", dest.GetHash()), "interface", networkIface.GetName())

	l := NewLink(dest, transport, networkIface, nil, nil)
	l.status.Store(int32(StatusPending))
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
		debug.Log(debug.DebugError, "Failed to handle link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		return nil, err
	}

	go l.startWatchdog()

	debug.Log(debug.DebugInfo, "Link established for incoming request", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return l, nil
}

func (l *Link) Establish() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	startTime := time.Now()
	debug.Log(debug.DebugInfo, "Establishing link", "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))

	if l.status.Load() != int32(StatusPending) {
		debug.Log(debug.DebugInfo, "Cannot establish link: invalid status", "status", l.status.Load())
		return errors.New("link already established or failed")
	}

	if l.destination == nil {
		return errors.New("destination is nil")
	}

	l.initiator = true
	l.status.Store(int32(StatusPending))
	l.requestTime = time.Now()

	if err := l.SendLinkRequest(); err != nil {
		l.markInitiatorEstablishmentFailedLocked()
		debug.Log(debug.DebugError, "Failed to send link request", "error", err, "elapsed", time.Since(startTime).Seconds())
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
			l.registerLinkPath()
		}
	}

	go l.startWatchdog()

	debug.Log(debug.DebugInfo, "Link establishment initiated", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return nil
}

// registerLinkPath copies the destination's transport path for this link's
// link_id, so outgoing link packets get the same multi-hop wrapping as
// destination-addressed packets.
func (l *Link) registerLinkPath() {
	if l.transport == nil || l.networkInterface == nil {
		return
	}

	var nextHop []byte
	var hops uint8

	if l.destination != nil {
		destHash := l.destination.GetHash()
		if h := l.transport.HopsTo(destHash); h > 0 && h < HopCountUnreachable {
			hops = h
		}
		if nh := l.transport.NextHop(destHash); len(nh) > 0 {
			nextHop = nh
		}
	}

	l.transport.UpdatePath(l.linkID, nextHop, l.networkInterface.GetName(), hops)
}

func (l *Link) Identify(id *identity.Identity) error {
	if !l.IsActive() {
		return errors.New("link not active")
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
		debug.Log(debug.DebugInfo, "Invalid identification data length", "length", len(data))
		return errors.New("invalid identification data length")
	}

	pubKey := data[:pubKeySize]
	signature := data[pubKeySize:]

	debug.Log(debug.DebugVerbose, "Processing identification from public key", "public_key", fmt.Sprintf("%x", pubKey[:8]))

	remoteIdentity := identity.FromPublicKey(pubKey)
	if remoteIdentity == nil {
		debug.Log(debug.DebugInfo, "Invalid remote identity from public key", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		return errors.New("invalid remote identity")
	}

	signData := append(l.linkID, pubKey...)
	if !remoteIdentity.Verify(signData, signature) {
		debug.Log(debug.DebugInfo, "Invalid signature from remote identity", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		return errors.New("invalid signature")
	}

	debug.Log(debug.DebugVerbose, "Remote identity verified successfully", "public_key", fmt.Sprintf("%x", pubKey[:8]))
	l.remoteIdentity = remoteIdentity

	if l.identifiedCallback != nil {
		debug.Log(debug.DebugVerbose, "Executing identified callback for remote identity", "public_key", fmt.Sprintf("%x", pubKey[:8]))
		l.identifiedCallback(l, remoteIdentity)
	}

	return nil
}

func (l *Link) Request(path string, data any, timeout time.Duration) (*RequestReceipt, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.status.Load() != int32(StatusActive) {
		return nil, errors.New("link not active")
	}

	pathHash := identity.TruncatedHash([]byte(path))
	requestData := []any{time.Now().Unix(), pathHash, data}
	packedRequest, err := msgpack.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to pack request: %w", err)
	}

	if timeout <= 0 {
		timeout = time.Duration(l.rtt*TrafficTimeoutFactor*float64(time.Second)) + time.Duration(resource.ResponseMaxGraceTime*1.125*float64(time.Second))
	}

	if len(packedRequest) <= l.mdu {
		reqPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   0,
			Context:         packet.ContextRequest,
			ContextFlag:     packet.FlagUnset,
			Hops:            0,
			DestinationType: DestTypeLink,
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

		debug.Log(debug.DebugInfo, "Sending request", "path", path, "request_id", fmt.Sprintf("%x", requestID))
		if err := l.transport.SendPacket(reqPkt); err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		receipt := &RequestReceipt{
			link:      l,
			requestID: requestID,
			status:    StatusPending,
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
	link          *Link
	mutex         sync.RWMutex
	requestID     []byte
	status        byte
	sentAt        time.Time
	receivedAt    time.Time
	response      []byte
	metadata      map[string]any
	timeout       time.Duration
	bytesReceived int64
	totalBytes    int64
	responseCb    func(*RequestReceipt)
	failedCb      func(*RequestReceipt)
	progressCb    func(*RequestReceipt)
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

// GetMetadata returns the metadata attached to a response delivered as a
// resource transfer (e.g. a file's name in nomadnetwork /file/ requests).
// It returns nil if the response carried no metadata.
func (r *RequestReceipt) GetMetadata() map[string]any {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.metadata
}

// Progress returns how many bytes of the response have arrived so far and
// the total number of bytes expected, for responses delivered as a resource
// transfer (e.g. large /file/ downloads). total is 0 until the resource
// advertisement carrying the transfer size has been received; both values
// are 0 for responses that never go through a resource transfer.
func (r *RequestReceipt) Progress() (received int64, total int64) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.bytesReceived, r.totalBytes
}

func (r *RequestReceipt) GetResponseTime() float64 {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.receivedAt.IsZero() {
		return 0.0
	}
	return r.receivedAt.Sub(r.sentAt).Seconds()
}

func (r *RequestReceipt) Concluded() bool {
	status := r.GetStatus()
	return status == StatusActive || status == StatusFailed
}

func (r *RequestReceipt) startTimeout() {
	time.Sleep(r.timeout)
	r.mutex.Lock()
	if r.status == StatusPending {
		r.status = StatusFailed
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
		return 0.0
	}
	return l.rssi
}

func (l *Link) GetSNR() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return 0.0
	}
	return l.snr
}

func (l *Link) GetQ() float64 {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.trackPhyStats {
		return 0.0
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
		return 0.0
	}
	return time.Since(l.establishedAt).Seconds()
}

// NoInboundFor returns the seconds elapsed since the last inbound packet.
func (l *Link) NoInboundFor() float64 {
	ns := l.lastInboundNs.Load()
	if ns == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, ns)).Seconds()
}

// NoOutboundFor returns the seconds elapsed since the last outbound packet.
func (l *Link) NoOutboundFor() float64 {
	ns := l.lastOutboundNs.Load()
	if ns == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, ns)).Seconds()
}

// NoDataFor returns the seconds since the most recent data packet (sent or received).
func (l *Link) NoDataFor() float64 {
	rxNs := l.lastDataReceivedNs.Load()
	txNs := l.lastDataSentNs.Load()
	last := max(txNs, rxNs)
	if last == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, last)).Seconds()
}

// InactiveFor returns the seconds since the most recent inbound or outbound packet.
func (l *Link) InactiveFor() float64 {
	inNs := l.lastInboundNs.Load()
	outNs := l.lastOutboundNs.Load()
	last := max(outNs, inNs)
	if last == 0 {
		return 0.0
	}
	return time.Since(time.Unix(0, last)).Seconds()
}

// nsToTime converts a UnixNano timestamp (0 means zero time) into a time.Time.
func nsToTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (l *Link) recordOutbound() {
	l.lastOutboundNs.Store(time.Now().UnixNano())
}

func (l *Link) recordOutboundData() {
	now := time.Now().UnixNano()
	l.lastOutboundNs.Store(now)
	l.lastDataSentNs.Store(now)
}

func (l *Link) recordInbound(isData bool) {
	now := time.Now().UnixNano()
	l.lastInboundNs.Store(now)
	if isData {
		l.lastDataReceivedNs.Store(now)
	}
}

func (l *Link) GetRemoteIdentity() *identity.Identity {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.remoteIdentity
}

func (l *Link) Teardown() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.status.Load() == int32(StatusActive) {
		_ = l.sendTeardownPacket() // #nosec G104 - best effort notification to peer
		l.status.Store(int32(StatusClosed))
		if l.transport != nil && len(l.linkID) > 0 {
			l.transport.UnregisterLink(l.linkID)
		}
		if l.closedCallback != nil {
			l.closedCallback(l)
		}
	}
	l.resetIncomingResource()
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
	l.packetCbMu.Lock()
	l.packetCallback = callback
	l.packetCbMu.Unlock()

	l.pendingPlainMu.Lock()
	data := l.pendingPlainData
	l.pendingPlainData = nil
	l.pendingPlainMu.Unlock()
	if callback != nil && len(data) > 0 {
		callback(data, nil)
	}
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
	if strategy != AcceptNone && strategy != AcceptAll && strategy != AcceptApp {
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

	if l.status.Load() != int32(StatusActive) {
		debug.Log(debug.DebugInfo, "Cannot send packet: link not active", "status", l.status.Load())
		return errors.New("link not active")
	}

	debug.Log(debug.DebugVerbose, "Encrypting packet", "bytes", len(data), "context", fmt.Sprintf("0x%02x", context))
	var wireData []byte
	var err error
	if context == packet.ContextResource || context == packet.ContextCacheReq {
		wireData = data
	} else {
		wireData, err = l.encrypt(data)
		if err != nil {
			debug.Log(debug.DebugInfo, "Failed to encrypt packet", "error", err)
			return err
		}
	}

	p := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         context,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		Data:            wireData,
		CreateReceipt:   false,
	}

	if err := p.Pack(); err != nil {
		return err
	}

	debug.Log(debug.DebugVerbose, "Sending encrypted packet", "bytes", len(wireData))
	l.recordOutboundData()

	return l.transport.SendPacket(p)
}

func (l *Link) HandleInbound(pkt *packet.Packet) error {
	if pkt.PacketType == packet.PacketTypeData {
		l.mutex.Lock()
		l.watchdogLock = true
		if l.status.Load() == int32(StatusClosed) {
			debug.Log(debug.DebugVerbose, "Ignoring packet for closed link", "link_id", fmt.Sprintf("%x", l.linkID))
			l.watchdogLock = false
			l.mutex.Unlock()
			return nil
		}

		l.recordInbound(pkt.Context != packet.ContextKeepalive)

		if l.status.Load() == int32(StatusStale) {
			l.status.Store(int32(StatusActive))
		}

		l.watchdogLock = false
		l.mutex.Unlock()
		return l.handleDataPacket(pkt)
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.watchdogLock = true
	defer func() {
		l.watchdogLock = false
	}()

	if l.status.Load() == int32(StatusClosed) {
		debug.Log(debug.DebugVerbose, "Ignoring packet for closed link", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil
	}

	l.recordInbound(pkt.Context != packet.ContextKeepalive)

	if l.status.Load() == int32(StatusStale) {
		l.status.Store(int32(StatusActive))
	}

	if pkt.PacketType == packet.PacketTypeProof {
		if pkt.Context == packet.ContextLRProof {
			return l.handleLinkProof(pkt, l.networkInterface)
		} else if pkt.Context == packet.ContextLRRTT {
			return l.handleRTTPacket(pkt)
		} else if pkt.Context == packet.ContextResourcePRF {
			return l.handleResourceProof(pkt)
		}
	}

	return nil
}

func (l *Link) deliverOrQueuePlainPacket(plaintext []byte, pkt *packet.Packet) {
	l.packetCbMu.RLock()
	cb := l.packetCallback
	l.packetCbMu.RUnlock()
	if cb != nil {
		data := append([]byte(nil), plaintext...)
		go func() {
			cb(data, pkt)
		}()
		return
	}
	l.pendingPlainMu.Lock()
	l.pendingPlainData = append([]byte(nil), plaintext...)
	l.pendingPlainMu.Unlock()
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
	if len(pkt.Data) < sha256.Size {
		return nil
	}
	resourceHash := pkt.Data[:sha256.Size]

	l.outgoingMu.Lock()
	out := l.outgoingRes
	l.outgoingMu.Unlock()
	if out == nil {
		return nil
	}
	if !bytes.Equal(out.GetHash(), resourceHash) {
		return nil
	}

	debug.Log(debug.DebugInfo, "Outgoing resource proof received", "resource_hash", fmt.Sprintf("%x", resourceHash))
	l.signalOutgoingResourceComplete()
	return nil
}

func (l *Link) handleDataPacket(pkt *packet.Packet) error {
	st := l.status.Load()
	if st != int32(StatusActive) && st != int32(StatusHandshake) {
		return errors.New("link not active")
	}

	if pkt.Context == packet.ContextLRRTT && st == int32(StatusHandshake) && !l.initiator {
		debug.Log(debug.DebugInfo, "RTT packet detected in handleDataPacket, routing to handleRTTPacket", "link_id", fmt.Sprintf("%x", l.linkID))
		return l.handleRTTPacket(pkt)
	}

	var plaintext []byte
	var err error

	if l.sessionKey != nil {
		if pkt.Context == packet.ContextResource {
			plaintext = pkt.Data
		} else if pkt.Context == packet.ContextCacheReq {
			plaintext = pkt.Data
		} else {
			minEnc := aes.BlockSize + aes.BlockSize + 32
			if pkt.Context == packet.ContextKeepalive && len(pkt.Data) < minEnc {
				plaintext = pkt.Data
			} else {
				plaintext, err = l.decrypt(pkt.Data)
				if err != nil {
					debug.Log(debug.DebugInfo, "Failed to decrypt packet", "error", err, "context", fmt.Sprintf("0x%02x", pkt.Context), "link_id", fmt.Sprintf("%x", l.linkID))
					return err
				}
			}
		}
	} else {
		plaintext = pkt.Data
	}

	switch pkt.Context {
	case packet.ContextNone:
		l.deliverOrQueuePlainPacket(plaintext, pkt)
	case packet.ContextRequest:
		return l.handleRequest(plaintext, pkt)
	case packet.ContextResponse:
		return l.handleResponse(plaintext)
	case packet.ContextLinkIdentify:
		return l.HandleIdentification(plaintext)
	case packet.ContextKeepalive:
		if !l.initiator && len(plaintext) == 1 && plaintext[0] == KeepaliveRequestByte {
			keepaliveResp := []byte{KeepaliveResponseByte}
			keepalivePkt := &packet.Packet{
				HeaderType:      packet.HeaderType1,
				PacketType:      packet.PacketTypeData,
				TransportType:   0,
				Context:         packet.ContextKeepalive,
				ContextFlag:     packet.FlagUnset,
				Hops:            0,
				DestinationType: DestTypeLink,
				DestinationHash: l.linkID,
				Data:            keepaliveResp,
				CreateReceipt:   false,
			}
			if err := keepalivePkt.Pack(); err != nil {
				return err
			}
			l.recordOutbound()
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
		debug.Log(debug.DebugInfo, "Failed to unpack resource advertisement", "error", err)
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
			debug.Log(debug.DebugInfo, "Received response resource advertisement for unknown request", "request_id", fmt.Sprintf("%x", requestID))
			return nil
		}

		l.incomingMu.Lock()
		l.incomingResourceRequest = matched
		l.incomingMu.Unlock()

		matched.mutex.Lock()
		matched.totalBytes = adv.TransferSize
		matched.mutex.Unlock()

		if err := l.beginIncomingResource(adv); err != nil {
			debug.Log(debug.DebugInfo, "Failed to begin incoming response resource", "error", err)
			l.incomingMu.Lock()
			l.incomingResourceRequest = nil
			l.incomingMu.Unlock()
			return err
		}
		return nil
	}

	if l.resourceStrategy == AcceptNone {
		_ = l.rejectResource(adv.Hash) // #nosec G104 - best effort resource rejection
		debug.Log(debug.DebugInfo, "Resource advertisement rejected (AcceptNone)")
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
			debug.Log(debug.DebugInfo, "Failed to begin incoming resource", "error", err)
			return err
		}
		if l.resourceStartedCallback != nil {
			l.resourceStartedCallback(adv)
		}
	} else {
		_ = l.rejectResource(adv.Hash) // #nosec G104 - best effort resource rejection
		debug.Log(debug.DebugInfo, "Resource advertisement rejected")
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
	for _, pi := range partIndexes {
		slice := out.OutboundCiphertextSlice(pi, partSDU)
		if len(slice) == 0 {
			continue
		}
		if err := l.SendPacketWithContext(slice, packet.ContextResource); err != nil {
			return
		}
		_ = out.MarkOutboundPartSent(pi)
	}
	if out.OutboundTransferComplete() {
		if err := l.SendPacketWithContext(nil, packet.ContextResource); err != nil {
			return
		}
	}
}

func chooseHashmapUpdateSegment(out *resource.Resource, sdu int, anchorHash []byte, receiverMinPart int) (int, int, bool) {
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
	searchStart := receiverMinPart
	if searchStart >= totalParts {
		searchStart = 0
	}
	searchEnd := min(searchStart+resource.CollisionGuardSize, totalParts)

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

	segment := (target + 1) / entries
	if segment <= 0 {
		return 0, 0, false
	}
	nextMin := target + 1
	return segment, nextMin, true
}

func (l *Link) sendResourceHashmapUpdate(out *resource.Resource, sdu int, anchorHash []byte, receiverMinPart int) (int, error) {
	segment, nextMin, ok := chooseHashmapUpdateSegment(out, sdu, anchorHash, receiverMinPart)
	if !ok {
		return -1, nil
	}
	hashmap := out.HashmapSegment(sdu, segment)
	if len(hashmap) == 0 {
		return -1, nil
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
	// HMU loss can deadlock receivers that pause new part requests while waiting.
	// Emit a best-effort immediate duplicate to improve delivery odds on lossy links.
	_ = l.SendPacketWithContext(payload, packet.ContextResourceHMU)
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
		if pi < 0 {
			candidates := out.PartIndicesForMapHash(mh)
			for _, idx := range candidates {
				if _, used := usedPartIndexes[idx]; used {
					continue
				}
				if !out.IsOutboundPartSent(idx) {
					pi = idx
					break
				}
			}
		}
		if pi < 0 {
			candidates := out.PartIndicesForMapHash(mh)
			for _, idx := range candidates {
				if _, used := usedPartIndexes[idx]; !used {
					pi = idx
					break
				}
			}
		}
		if pi < 0 {
			candidates := out.PartIndicesForMapHash(mh)
			if len(candidates) == 0 {
				continue
			}
			pi = candidates[0]
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
	_, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}
	l.resetIncomingResource()
	return nil
}

func (l *Link) handleResourceReject(pkt *packet.Packet) error {
	_, err := l.decrypt(pkt.Data)
	if err != nil {
		return err
	}
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

func (l *Link) handleRequest(plaintext []byte, pkt *packet.Packet) error {
	if l.destination == nil {
		return errors.New("no destination for request handling")
	}

	var requestData []any
	if err := msgpack.Unmarshal(plaintext, &requestData); err != nil {
		return fmt.Errorf("failed to unpack request: %w", err)
	}

	if len(requestData) < MinRequestDataLen {
		return errors.New("invalid request format")
	}

	requestedAtFloat, ok := requestData[0].(float64)
	if !ok {
		requestedAtInt, ok := requestData[0].(int64)
		if !ok {
			return fmt.Errorf("invalid requested_at type: %T", requestData[0])
		}
		requestedAtFloat = float64(requestedAtInt)
	}
	requestedAt := time.Unix(int64(requestedAtFloat), 0)

	pathHash, ok := requestData[1].([]byte)
	if !ok {
		return fmt.Errorf("invalid path_hash type: %T", requestData[1])
	}

	var requestPayload []byte
	if requestData[2] != nil {
		switch payload := requestData[2].(type) {
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

	debug.Log(debug.DebugInfo, "Handling request", "path_hash", fmt.Sprintf("%x", pathHash), "request_id", fmt.Sprintf("%x", requestID))

	if l.destination != nil {
		handler := l.destination.GetRequestHandler(pathHash)
		if handler != nil {
			response := handler(pathHash, requestPayload, requestID, l.linkID, l.remoteIdentity, requestedAt)
			if response != nil {
				return l.sendResponse(requestID, response)
			}
		} else {
			debug.Log(debug.DebugVerbose, "No handler found for path", "path_hash", fmt.Sprintf("%x", pathHash))
		}
	}

	return nil
}

func (l *Link) handleResponse(plaintext []byte) error {
	var responseData []any
	if err := msgpack.Unmarshal(plaintext, &responseData); err != nil {
		return fmt.Errorf("failed to unpack response: %w", err)
	}

	if len(responseData) < MinResponseDataLen {
		return errors.New("invalid response format")
	}

	requestIDRaw, ok := responseData[0].([]byte)
	if !ok {
		return errors.New("invalid response format: request id is not bytes")
	}
	var responsePayload []byte
	switch p := responseData[1].(type) {
	case []byte:
		responsePayload = p
	case string:
		responsePayload = []byte(p)
	default:
		return errors.New("invalid response format: response payload is not bytes or string")
	}
	requestID := requestIDRaw

	l.requestMutex.Lock()
	for i, req := range l.pendingRequests {
		if string(req.requestID) == string(requestID) {
			req.mutex.Lock()
			req.status = StatusActive
			req.response = responsePayload
			req.receivedAt = time.Now()
			req.bytesReceived = int64(len(responsePayload))
			req.totalBytes = int64(len(responsePayload))
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

	l.mutex.RLock()
	mdu := l.mdu
	l.mutex.RUnlock()

	if len(packedResponse) <= mdu {
		encrypted, err := l.encrypt(packedResponse)
		if err != nil {
			return err
		}

		respPkt := &packet.Packet{
			HeaderType:      packet.HeaderType1,
			PacketType:      packet.PacketTypeData,
			TransportType:   0,
			Context:         packet.ContextResponse,
			ContextFlag:     packet.FlagUnset,
			Hops:            0,
			DestinationType: DestTypeLink,
			DestinationHash: l.linkID,
			Data:            encrypted,
			CreateReceipt:   false,
		}

		if err := respPkt.Pack(); err != nil {
			return err
		}

		l.recordOutboundData()

		debug.Log(debug.DebugInfo, "Sending response", "request_id", fmt.Sprintf("%x", requestID), "response_len", len(encrypted))
		return l.transport.SendPacket(respPkt)
	}

	res, err := resource.New(packedResponse, false)
	if err != nil {
		return fmt.Errorf("failed to create response resource: %w", err)
	}
	res.SetRequestID(requestID)
	res.SetIsResponse(true)

	debug.Log(debug.DebugInfo, "Sending response as resource", "request_id", fmt.Sprintf("%x", requestID), "packed_len", len(packedResponse), "mdu", mdu)
	go func() {
		if err := l.SendResource(res); err != nil {
			debug.Log(debug.DebugError, "Failed to send response resource", "request_id", fmt.Sprintf("%x", requestID), "error", err)
		}
	}()
	return nil
}

func (l *Link) handleRTTPacket(pkt *packet.Packet) error {
	if !l.initiator {
		measuredRTT := time.Since(l.requestTime).Seconds()
		debug.Log(debug.DebugInfo, "Handling RTT packet (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "has_session_key", l.sessionKey != nil, "status", l.status.Load(), "data_len", len(pkt.Data))
		plaintext, err := l.decrypt(pkt.Data)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to decrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			return err
		}
		debug.Log(debug.DebugInfo, "RTT packet decrypted successfully", "plaintext_len", len(plaintext), "link_id", fmt.Sprintf("%x", l.linkID))

		rtt, err := parseRTTPayloadSeconds(plaintext)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to decode RTT payload", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			return err
		}

		l.mutex.Lock()
		l.rtt = maxFloat(measuredRTT, rtt)
		l.establishedAt = time.Now()
		if l.rtt > 0 {
			l.updateKeepaliveLocked()
		}
		logRtt := l.rtt
		l.mutex.Unlock()

		l.status.Store(int32(StatusActive))

		if l.transport != nil {
			l.transport.RegisterLink(l.linkID, l)
			if l.networkInterface != nil {
				l.registerLinkPath()
			}
		}

		if l.establishedCallback != nil {
			go l.establishedCallback(l)
		}

		establishmentElapsed := time.Since(l.requestTime).Seconds()
		debug.Log(debug.DebugInfo, "Link established (responder) after RTT", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt), "total_elapsed", fmt.Sprintf("%.3fs", establishmentElapsed))
	}
	return nil
}

func parseRTTPayloadSeconds(payload []byte) (float64, error) {
	if len(payload) == 0 {
		return 0, errors.New("empty RTT payload")
	}
	if payload[0] != MsgpackFloat32Code && payload[0] != MsgpackFloat64Code {
		return 0, errors.New("RTT payload is not msgpack float")
	}

	var rtt float64
	if err := msgpack.Unmarshal(payload, &rtt); err != nil {
		return 0, fmt.Errorf("invalid msgpack RTT payload: %w", err)
	}
	if rtt < 0 {
		return 0, errors.New("negative RTT payload")
	}
	return rtt, nil
}

func (l *Link) updateKeepaliveLocked() {
	if l.rtt <= 0 {
		return
	}

	keepaliveMax := float64(Keepalive)

	calculatedKeepalive := l.rtt * (keepaliveMax / KeepaliveMaxRTT)
	if calculatedKeepalive > keepaliveMax {
		calculatedKeepalive = keepaliveMax
	}
	if calculatedKeepalive < KeepaliveMinSec {
		calculatedKeepalive = KeepaliveMinSec
	}

	l.keepalive = time.Duration(calculatedKeepalive * float64(time.Second))
	l.staleTime = time.Duration(float64(l.keepalive) * float64(2))
}

func (l *Link) handleLinkProof(pkt *packet.Packet, networkIface common.NetworkInterface) error {
	if l.initiator {
		return l.ValidateLinkProof(pkt, networkIface)
	}
	return nil
}

func (l *Link) handleTeardown(plaintext []byte) error {
	if len(plaintext) == len(l.linkID) && string(plaintext) == string(l.linkID) {
		l.status.Store(int32(StatusClosed))
		l.teardownReason = StatusFailed
		if l.transport != nil && len(l.linkID) > 0 {
			l.transport.UnregisterLink(l.linkID)
		}
		if l.initiator && l.establishedAt.IsZero() {
			l.invalidateTransportPathAfterInitiatorFailure()
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
		debug.Log(debug.DebugError, "Decrypt failed: no session keys", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil, errors.New("no session keys available")
	}

	// Minimum length: IV(16) + at least one block(16) + HMAC(32) = 64 bytes
	if len(data) < aes.BlockSize+aes.BlockSize+32 {
		debug.Log(debug.DebugError, "Decrypt failed: data too short", "length", len(data))
		return nil, errors.New("data too short")
	}

	// Split into [IV + Ciphertext] and [HMAC]
	signedParts := data[:len(data)-32]
	receivedMac := data[len(data)-32:]

	// Verify HMAC
	h := hmac.New(sha256.New, l.hmacKey)
	h.Write(signedParts)
	expectedMac := h.Sum(nil)
	if !hmac.Equal(receivedMac, expectedMac) {
		debug.Log(debug.DebugError, "Decrypt failed: HMAC mismatch", "link_id", fmt.Sprintf("%x", l.linkID))
		return nil, errors.New("HMAC verification failed")
	}

	plaintext, err := cryptography.DecryptAES256CBC(l.sessionKey, signedParts)
	if err != nil {
		debug.Log(debug.DebugError, "Decrypt failed", "link_id", fmt.Sprintf("%x", l.linkID), "error", err)
		return nil, err
	}

	return plaintext, nil
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
	case int32(StatusPending):
		return StatusPending
	case int32(StatusHandshake):
		return StatusHandshake
	case int32(StatusActive):
		return StatusActive
	case int32(StatusStale):
		return StatusStale
	case int32(StatusClosed):
		return StatusClosed
	case int32(StatusFailed):
		return StatusFailed
	default:
		return StatusFailed
	}
}

func (l *Link) Send(data []byte) any {
	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextChannel,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
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

	l.recordOutbound()
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

// LinkedNetworkInterface implements [transport.LinkInterface] for iface teardown.
func (l *Link) LinkedNetworkInterface() common.NetworkInterface {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.networkInterface
}

func (l *Link) IsActive() bool {
	return l.GetStatus() == StatusActive
}

func (l *Link) SendResource(res *resource.Resource) error {
	l.resourceSendMu.Lock()
	defer l.resourceSendMu.Unlock()

	l.mutex.Lock()
	if l.status.Load() != int32(StatusActive) {
		l.teardownReason = StatusFailed
		l.mutex.Unlock()
		return errors.New("link not active")
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

func (l *Link) maintainLink() {
	ticker := time.NewTicker(time.Second * Keepalive)
	defer ticker.Stop()

	for range ticker.C {
		if l.status.Load() != int32(StatusActive) {
			return
		}

		inactiveTime := l.InactiveFor()
		if inactiveTime > float64(StaleTime) {
			l.mutex.Lock()
			l.teardownReason = StatusFailed
			l.mutex.Unlock()
			l.Teardown()
			return
		}

		noDataTime := l.NoDataFor()
		if noDataTime > float64(Keepalive) {
			l.mutex.Lock()
			err := l.sendKeepalive()
			if err != nil {
				l.teardownReason = StatusFailed
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

func (l *Link) HandleProofRequest(packet *packet.Packet) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

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

func (l *Link) startWatchdog() {
	if l.watchdogActive {
		return
	}

	l.watchdogActive = true
	go l.watchdog()
}

func (l *Link) watchdog() {
	for l.GetStatus() != StatusClosed {
		l.mutex.Lock()
		if l.watchdogLock {
			rttWait := WatchdogMinSleep
			if l.rtt > 0.0 {
				rttWait = l.rtt
			}
			if rttWait < WatchdogMinSleep {
				rttWait = WatchdogMinSleep
			}
			l.mutex.Unlock()
			time.Sleep(time.Duration(rttWait * float64(time.Second)))
			continue
		}

		var sleepTime = WatchdogInterval

		if l.status.Load() == int32(StatusPending) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				debug.Log(debug.DebugInfo, "Link establishment timed out", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load())
				l.status.Store(int32(StatusClosed))
				l.teardownReason = StatusFailed
				if l.transport != nil && len(l.linkID) > 0 {
					l.transport.UnregisterLink(l.linkID)
				}
				if l.initiator {
					l.invalidateTransportPathAfterInitiatorFailure()
				}
				if l.closedCallback != nil {
					l.closedCallback(l)
				}
				sleepTime = 0.001
			}
		} else if l.status.Load() == int32(StatusHandshake) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				elapsed := time.Since(l.requestTime).Seconds()
				if l.initiator {
					debug.Log(debug.DebugInfo, "Timeout waiting for link request proof", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				} else {
					debug.Log(debug.DebugInfo, "Timeout waiting for RTT packet from link initiator", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				}
				l.status.Store(int32(StatusClosed))
				l.teardownReason = StatusFailed
				if l.transport != nil && len(l.linkID) > 0 {
					l.transport.UnregisterLink(l.linkID)
				}
				if l.initiator {
					l.invalidateTransportPathAfterInitiatorFailure()
				}
				if l.closedCallback != nil {
					l.closedCallback(l)
				}
				sleepTime = 0.001
			}
		} else if l.status.Load() == int32(StatusActive) {
			activatedAt := l.establishedAt
			if activatedAt.IsZero() {
				activatedAt = time.Time{}
			}
			lastInbound := nsToTime(l.lastInboundNs.Load())
			lastOutbound := nsToTime(l.lastOutboundNs.Load())
			lastDataSent := nsToTime(l.lastDataSentNs.Load())
			lastActivity := lastInbound
			if lastOutbound.After(lastActivity) {
				lastActivity = lastOutbound
			}
			if lastDataSent.After(lastActivity) {
				lastActivity = lastDataSent
			}
			if lastActivity.Before(activatedAt) {
				lastActivity = activatedAt
			}
			now := time.Now()

			if now.After(lastActivity.Add(l.keepalive)) {
				if l.initiator {
					lastKeepalive := lastOutbound
					if now.After(lastKeepalive.Add(l.keepalive)) {
						_ = l.sendKeepalive() // #nosec G104 - best effort keepalive
					}
				}

				if now.After(lastActivity.Add(l.staleTime)) {
					sleepTime = l.rtt*KeepaliveTimeoutFactor + StaleGrace
					l.status.Store(int32(StatusStale))
				} else {
					sleepTime = float64(l.keepalive) / float64(time.Second)
				}
			} else {
				nextKeepalive := lastActivity.Add(l.keepalive)
				sleepTime = time.Until(nextKeepalive).Seconds()
			}
		} else if l.status.Load() == int32(StatusStale) {
			sleepTime = 0.001
			debug.Log(debug.DebugInfo, "Link marked stale, closing", "link_id", fmt.Sprintf("%x", l.linkID))
			_ = l.sendTeardownPacket() // #nosec G104 - best effort teardown
			l.status.Store(int32(StatusClosed))
			l.teardownReason = StatusFailed
			if l.closedCallback != nil {
				l.closedCallback(l)
			}
			sleepTime = 0.001
		}

		if sleepTime <= 0.0 {
			sleepTime = 0.1
		}
		if sleepTime > 5.0 {
			sleepTime = 5.0
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
		TransportType:   0,
		Context:         packet.ContextKeepalive,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
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
	l.recordOutbound()
	return l.transport.SendPacket(keepalivePkt)
}

func (l *Link) sendTeardownPacket() error {
	teardownPkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextLinkClose,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
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
	l.recordOutbound()
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

	pubKey, privKey, err := cryptography.GenerateSigningKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate Ed25519 keypair: %w", err)
	}
	l.sigPriv = privKey
	l.sigPub = pubKey

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

func (l *Link) SendLinkRequest() error {
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

	sendStartTime := time.Now()
	if err := l.transport.SendPacket(pkt); err != nil {
		debug.Log(debug.DebugError, "Failed to send link request", "error", err, "elapsed", time.Since(sendStartTime).Seconds())
		return fmt.Errorf("failed to send link request: %w", err)
	}

	debug.Log(debug.DebugInfo, "Link request sent", "link_id", fmt.Sprintf("%x", l.linkID), "send_elapsed", time.Since(sendStartTime).Seconds(), "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	return nil
}

func linkIDFromPacket(pkt *packet.Packet) []byte {
	hashablePart := []byte{pkt.Raw[0] & 0b00001111}

	if pkt.HeaderType == packet.HeaderType2 {
		dstLen := 16
		startIndex := dstLen + 2
		if len(pkt.Raw) > startIndex {
			hashablePart = append(hashablePart, pkt.Raw[startIndex:]...)
		}
	} else {
		if len(pkt.Raw) > 2 {
			hashablePart = append(hashablePart, pkt.Raw[2:]...)
		}
	}

	if len(pkt.Data) > ECPubSize {
		diff := len(pkt.Data) - ECPubSize
		if len(hashablePart) >= diff {
			hashablePart = hashablePart[:len(hashablePart)-diff]
		}
	}

	return identity.TruncatedHash(hashablePart)
}

func (l *Link) HandleLinkRequest(pkt *packet.Packet, ownerIdentity *identity.Identity) error {
	startTime := time.Now()
	debug.Log(debug.DebugInfo, "Handling incoming link request", "data_len", len(pkt.Data), "has_interface", l.networkInterface != nil, "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))
	if len(pkt.Data) < ECPubSize {
		return errors.New("link request data too short")
	}

	peerPub := pkt.Data[0:KeySize]
	peerSigPub := pkt.Data[KeySize:ECPubSize]

	l.peerPub = peerPub
	l.peerSigPub = peerSigPub
	l.linkID = linkIDFromPacket(pkt)
	l.initiator = false

	myPubStr := "not_generated_yet"
	if len(l.pub) >= 8 {
		myPubStr = fmt.Sprintf("%x", l.pub[:8])
	}
	debug.Log(debug.DebugInfo, "Link request processed (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "my_pub", myPubStr, "elapsed", time.Since(startTime).Seconds())

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

	debug.Log(debug.DebugInfo, "Ephemeral keys generated (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "my_pub", fmt.Sprintf("%x", l.pub[:8]), "peer_pub", fmt.Sprintf("%x", l.peerPub[:8]))

	if err := l.performHandshake(); err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	l.status.Store(int32(StatusHandshake))
	l.recordInbound(false)
	l.requestTime = time.Now()
	// Match reference responder behavior: establishment timeout is per-hop plus keepalive grace.
	// This prevents WAN/backbone proof/RTT races from being closed too aggressively.
	hops := max(int(pkt.Hops), 1)
	l.establishmentTimeout = time.Duration(float64(hops)*EstablishmentTimeoutPerHop*float64(time.Second)) + l.keepalive
	debug.Log(debug.DebugInfo, "Responder establishment timeout configured", "link_id", fmt.Sprintf("%x", l.linkID), "packet_hops", pkt.Hops, "effective_hops", hops, "timeout_sec", l.establishmentTimeout.Seconds())

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

	debug.Log(debug.DebugInfo, "Link proof sent (responder), waiting for RTT", "link_id", fmt.Sprintf("%x", l.linkID), "proof_send_elapsed", time.Since(proofStartTime).Seconds(), "total_elapsed", time.Since(startTime).Seconds())

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
	if len(l.peerPub) != KeySize {
		return errors.New("invalid peer public key length")
	}

	sharedSecret, err := cryptography.DeriveSharedSecret(l.prv, l.peerPub)
	if err != nil {
		return fmt.Errorf("ECDH failed: %w", err)
	}
	l.sharedKey = sharedSecret

	var derivedKeyLength int
	if l.mode == ModeAES128CBC {
		derivedKeyLength = 32
	} else if l.mode == ModeAES256CBC {
		derivedKeyLength = 64
	} else {
		return fmt.Errorf("invalid link mode: %d", l.mode)
	}

	derivedKey, err := cryptography.DeriveKey(l.sharedKey, l.linkID, nil, derivedKeyLength)
	if err != nil {
		return fmt.Errorf("HKDF failed: %w", err)
	}
	l.derivedKey = derivedKey

	if len(derivedKey) >= 64 {
		l.hmacKey = derivedKey[0:32]
		l.sessionKey = derivedKey[32:64]
		debug.Log(debug.DebugInfo, "Session keys derived", "link_id", fmt.Sprintf("%x", l.linkID), "mode", l.mode, "initiator", l.initiator, "hmac_key", fmt.Sprintf("%x", l.hmacKey[:8]), "session_key", fmt.Sprintf("%x", l.sessionKey[:8]))
	} else if len(derivedKey) >= 32 {
		l.hmacKey = derivedKey[0:16]
		l.sessionKey = derivedKey[16:32]
	}

	l.status.Store(int32(StatusHandshake))
	debug.Log(debug.DebugVerbose, "Handshake completed", "key_material_bytes", len(derivedKey), "shared_key", fmt.Sprintf("%x", l.sharedKey[:8]), "link_id", fmt.Sprintf("%x", l.linkID))
	return nil
}

func (l *Link) sendLinkProof(ownerIdentity *identity.Identity) error {
	debug.Log(debug.DebugError, "Generating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "initiator", l.initiator, "has_interface", l.networkInterface != nil)

	proofPkt, err := l.GenerateLinkProof(ownerIdentity)
	if err != nil {
		return err
	}

	debug.Log(debug.DebugError, "Link proof packet created", "dest_hash", fmt.Sprintf("%x", proofPkt.DestinationHash), "packet_type", fmt.Sprintf("0x%02x", proofPkt.PacketType))

	// For responder links (not initiator), send proof directly through the receiving interface
	if !l.initiator && l.networkInterface != nil {
		if err := proofPkt.Pack(); err != nil {
			return fmt.Errorf("failed to pack proof packet: %w", err)
		}

		debug.Log(debug.DebugError, "Sending proof through interface", "raw_len", len(proofPkt.Raw), "interface", l.networkInterface.GetName())

		if err := l.networkInterface.Send(proofPkt.Raw, ""); err != nil {
			return fmt.Errorf("failed to send link proof through interface: %w", err)
		}
		debug.Log(debug.DebugError, "Link proof sent through interface", "link_id", fmt.Sprintf("%x", l.linkID), "interface", l.networkInterface.GetName())
		return nil
	}

	// For initiator links, use transport (path lookup)
	if l.transport != nil {
		if err := l.transport.SendPacket(proofPkt); err != nil {
			return fmt.Errorf("failed to send link proof: %w", err)
		}
		debug.Log(debug.DebugInfo, "Link proof sent", "link_id", fmt.Sprintf("%x", l.linkID))
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
	startTime := time.Now()
	debug.Log(debug.DebugInfo, "Validating link proof", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load(), "initiator", l.initiator, "has_interface", networkIface != nil, "proof_data_len", len(pkt.Data))
	st := l.status.Load()
	if st != int32(StatusPending) && st != int32(StatusHandshake) {
		return fmt.Errorf("invalid link status for proof validation: %d", l.status.Load())
	}

	if len(pkt.Data) < identity.SigLength/8+KeySize {
		l.markInitiatorEstablishmentFailedLocked()
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

	l.peerPub = peerPub
	if l.destination != nil && l.destination.GetIdentity() != nil {
		destIdent := l.destination.GetIdentity()
		pubKey := destIdent.GetPublicKey()
		if len(pubKey) >= ECPubSize {
			l.peerSigPub = pubKey[KeySize:ECPubSize]
		}
	}

	signedData := make([]byte, 0, len(l.linkID)+KeySize+len(l.peerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, peerPub...)
	signedData = append(signedData, l.peerSigPub...)
	signedData = append(signedData, signalling...)

	first32Len := min(len(signedData), 32)
	debug.Log(debug.DebugInfo, "Constructed signed data for validation", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "peer_pub", fmt.Sprintf("%x", peerPub[:8]), "peer_sig_pub", fmt.Sprintf("%x", l.peerSigPub[:8]), "signalling", fmt.Sprintf("%x", signalling), "signed_data_len", len(signedData), "signed_data_first32", fmt.Sprintf("%x", signedData[:first32Len]))

	if l.destination == nil || l.destination.GetIdentity() == nil {
		l.markInitiatorEstablishmentFailedLocked()
		return errors.New("no destination identity for proof validation")
	}

	if !l.destination.GetIdentity().Verify(signedData, signature) {
		debug.Log(debug.DebugError, "Link proof signature validation failed", "link_id", fmt.Sprintf("%x", l.linkID[:8]), "signature", fmt.Sprintf("%x", signature[:8]), "signed_data", fmt.Sprintf("%x", signedData))
		l.markInitiatorEstablishmentFailedLocked()
		return errors.New("link proof signature validation failed")
	}
	debug.Log(debug.DebugInfo, "Link proof signature validated successfully", "link_id", fmt.Sprintf("%x", l.linkID[:8]))

	if err := l.performHandshake(); err != nil {
		l.markInitiatorEstablishmentFailedLocked()
		return fmt.Errorf("handshake failed: %w", err)
	}

	l.updateMDU()

	l.mutex.Lock()
	l.rtt = time.Since(l.requestTime).Seconds()
	l.establishedAt = time.Now()
	if l.rtt > 0 {
		l.updateKeepaliveLocked()
	}
	logRtt := l.rtt
	l.mutex.Unlock()

	l.status.Store(int32(StatusActive))

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

	encrypted, err := l.encrypt(rttData)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to encrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
	} else {
		rttPkt.Data = encrypted
		if err := rttPkt.Pack(); err != nil {
			debug.Log(debug.DebugError, "Failed to pack RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
		} else {
			debug.Log(debug.DebugInfo, "Sending RTT packet", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt), "packet_size", len(rttPkt.Raw))
			if err := l.transport.SendPacket(rttPkt); err != nil {
				debug.Log(debug.DebugError, "Failed to send RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			} else {
				l.recordOutbound()
				debug.Log(debug.DebugInfo, "RTT packet sent successfully", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt))
			}
		}
	}

	establishmentElapsed := time.Since(l.requestTime).Seconds()
	debug.Log(debug.DebugInfo, "Link established (initiator)", "link_id", fmt.Sprintf("%x", l.linkID), "rtt", fmt.Sprintf("%.3fs", logRtt), "total_elapsed", fmt.Sprintf("%.3fs", establishmentElapsed), "validation_elapsed", fmt.Sprintf("%.3fs", time.Since(startTime).Seconds()))

	if l.establishedCallback != nil {
		go l.establishedCallback(l)
	}

	return nil
}
