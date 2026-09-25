// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"errors"
	"fmt"
	"hash"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/channel"
	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
	"github.com/Quad4-Software/Reticulum-Go/pkg/destination"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
	"github.com/Quad4-Software/Reticulum-Go/pkg/pathfinder"
	"github.com/Quad4-Software/Reticulum-Go/pkg/resource"
	"github.com/Quad4-Software/Reticulum-Go/pkg/securemem"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
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

var errHMACVerificationFailed = errors.New("HMAC verification failed")

type Link struct {
	mutex              sync.RWMutex
	destination        *destination.Destination
	status             atomic.Int32
	networkInterface   common.NetworkInterface
	establishedAt      time.Time
	lastInboundNs      atomic.Int64
	lastOutboundNs     atomic.Int64
	lastKeepaliveNs    atomic.Int64
	lastDataReceivedNs atomic.Int64
	lastDataSentNs     atomic.Int64
	pathFinder         *pathfinder.PathFinder

	remoteIdentity *identity.Identity
	sessionKey     *securemem.Buf
	aesBlock       cipher.Block
	hmacSend       hash.Hash
	hmacRecv       hash.Hash
	hmacSendMu     sync.Mutex
	hmacRecvMu     sync.Mutex
	linkID         []byte

	rtt               float64
	establishmentRate float64

	establishedCallback func(*Link)
	closedCallback      func(*Link)
	packetCallback      func([]byte, *packet.Packet)
	packetCbMu          sync.RWMutex
	identifiedCallback  func(*Link, *identity.Identity)

	teardownReason byte
	hmacKey        *securemem.Buf
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

	watchdogLock   bool
	watchdogActive atomic.Bool
	// watchdogDone is closed by closeOnce so the watchdog exits promptly
	// instead of lingering for up to a 5s sleep after close.
	watchdogDone chan struct{}
	// linkDone is closed by closeOnce. Per-packet timeout and request
	// waiters select on it so teardown releases sleepers immediately.
	linkDone             chan struct{}
	establishmentTimeout time.Duration
	keepalive            time.Duration
	staleTime            time.Duration
	initiator            bool
	expectedHops         uint8
	rebalanced           time.Time

	prv           *securemem.Buf
	sigPriv       *securemem.Buf
	pub           []byte
	sigPub        ed25519.PublicKey
	peerPub       []byte
	peerSigPub    ed25519.PublicKey
	sharedKey     *securemem.Buf
	derivedKey    *securemem.Buf
	mode          byte
	mtu           int
	mdu           int
	requestTime   time.Time
	requestPacket *packet.Packet

	pendingRequests []*RequestReceipt
	requestMutex    sync.RWMutex

	channel      *channel.Channel
	channelMutex sync.RWMutex

	// channelReceipts tracks outstanding Channel.Send envelopes awaiting link proof.
	channelReceiptMu sync.Mutex
	channelReceipts  map[*packet.Packet]*packet.PacketReceipt

	incomingMu sync.Mutex
	incomingRx *incomingResourceAsm

	outgoingMu     sync.Mutex
	resourceSendMu sync.Mutex
	// resSendInflight bounds goroutines parked in SendResource. Each holds a
	// full resource copy, so a rapid requester could otherwise pile them up.
	resSendInflight         atomic.Int64
	outgoingRes             *resource.Resource
	outgoingReceiverMinPart int
	outgoingResCompleteChan chan struct{}
	outgoingDispatchMu      sync.Mutex

	pendingPlainMu   sync.Mutex
	pendingPlainData []byte

	// earlyChannel holds ContextChannel packets that arrive after handshake
	// keys exist but before promoteToActive. Python rnsh sends Version in that
	// window. Processing them before the established callback drops messages.
	earlyChannelMu sync.Mutex
	earlyChannel   []*packet.Packet
}

func NewLink(dest *destination.Destination, transport *transport.Transport, networkIface common.NetworkInterface, establishedCallback func(*Link), closedCallback func(*Link)) *Link {
	if dest == nil {
		debug.Log(debug.DebugError, common.MsgLinkNilDestination)
	}
	if transport == nil {
		debug.Log(debug.DebugError, common.MsgLinkNilTransport)
	}
	return &Link{
		destination:         dest,
		transport:           transport,
		networkInterface:    networkIface,
		establishedCallback: establishedCallback,
		closedCallback:      closedCallback,
		establishedAt:       time.Time{},
		pathFinder:          pathfinder.NewPathFinder(),

		watchdogLock:         false,
		establishmentTimeout: time.Duration(EstablishmentTimeoutPerHop * float64(time.Second)),
		keepalive:            time.Duration(Keepalive * float64(time.Second)),
		staleTime:            time.Duration(StaleTime * float64(time.Second)),
		initiator:            false,
		pendingRequests:      make([]*RequestReceipt, 0),
		linkDone:             make(chan struct{}),
	}
}
func (l *Link) Establish() error {
	l.mutex.Lock()
	startTime := time.Now()

	if l.status.Load() != int32(StatusPending) {
		debug.Log(debug.DebugWarning, common.MsgLinkAlreadySettled,
			"status", l.status.Load(),
			"hint", "wait for the established or closed callback, do not call Establish again")
		l.mutex.Unlock()
		return common.ErrLinkAlreadySettled
	}
	if !l.requestTime.IsZero() {
		debug.Log(debug.DebugWarning, common.MsgLinkEstablishBusy,
			"hint", "wait for the established callback, do not loop NewLink/Establish")
		l.mutex.Unlock()
		return common.ErrLinkEstablishBusy
	}

	if l.destination == nil {
		l.mutex.Unlock()
		return common.ErrLinkDestinationRequired
	}

	debug.Log(debug.DebugVerbose, "Establishing link", "dest_hash", fmt.Sprintf("%x", l.destination.GetHash()))

	if l.transport == nil {
		l.mutex.Unlock()
		return common.ErrLinkTransportRequired
	}

	destHash := l.destination.GetHash()
	if !l.transport.HasPath(destHash) {
		l.mutex.Unlock()
		return common.ErrLinkNoPathf(destHash)
	}

	if err := l.transport.TryBeginOutboundEstablish(destHash); err != nil {
		l.mutex.Unlock()
		return err
	}

	l.initiator = true
	l.status.Store(int32(StatusPending))
	l.requestTime = time.Now()
	l.expectedHops = l.transport.HopsTo(destHash)
	hops := int(l.expectedHops)
	if hops < 1 || l.expectedHops >= HopCountUnreachable {
		hops = 1
	}
	firstHop := l.transport.FirstHopTimeout(destHash)
	l.establishmentTimeout = time.Duration((firstHop + float64(hops)*EstablishmentTimeoutPerHop) * float64(time.Second))

	if err := l.prepareLinkRequestLocked(); err != nil {
		debug.Log(debug.DebugError, "Failed to prepare link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		l.requestTime = time.Time{}
		l.transport.EndOutboundEstablish(destHash)
		l.mutex.Unlock()
		return err
	}

	// Register before sending so an immediate link proof cannot race and miss.
	// The mutex is released before SendPacket because synchronous interfaces
	// may deliver the proof back into ValidateLinkProof on this goroutine.
	l.transport.RegisterLink(l.linkID, l)

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

	l.mutex.Unlock()

	if err := l.sendPreparedLinkRequest(); err != nil {
		l.mutex.Lock()
		l.markInitiatorEstablishmentFailedLocked()
		l.mutex.Unlock()
		debug.Log(debug.DebugError, "Failed to send link request", "error", err, "elapsed", time.Since(startTime).Seconds())
		return err
	}
	go l.startWatchdog()

	debug.Log(debug.DebugVerbose, "Link establishment initiated", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", time.Since(startTime).Seconds())
	return nil
}

// Reestablish resets a closed or failed link and starts a new establishment attempt.
func (l *Link) Reestablish() error {
	l.mutex.Lock()
	st := l.status.Load()
	if st == int32(StatusPending) || st == int32(StatusHandshake) || st == int32(StatusActive) {
		l.mutex.Unlock()
		return common.ErrLinkAlreadySettled
	}
	l.resetForReconnectLocked()
	l.mutex.Unlock()
	return l.Establish()
}
func (l *Link) resetForReconnectLocked() {
	l.status.Store(int32(StatusPending))
	l.remoteIdentity = nil
	l.closeAllSecretKeys()
	l.establishedAt = time.Time{}
	l.requestPacket = nil
	l.requestTime = time.Time{}
	l.teardownReason = 0
	// The stale linkID must leave the transport's link table. Otherwise a
	// replayed proof for the old ID can interfere with the fresh attempt.
	if l.transport != nil && len(l.linkID) > 0 {
		l.transport.UnregisterLink(l.linkID)
	}
	l.linkID = nil
	l.pub = nil
	l.sigPub = nil
	l.peerPub = nil
	l.peerSigPub = nil
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
	l.transport.MarkLinkPath(l.linkID)
}
func (l *Link) GetRemoteIdentity() *identity.Identity {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.remoteIdentity
}
func (l *Link) Teardown() {
	l.mutex.Lock()

	if l.status.Load() == int32(StatusClosed) {
		l.resetIncomingResource()
		l.mutex.Unlock()
		return
	}
	if !l.closeOnce(StatusClosed) {
		l.resetIncomingResource()
		l.mutex.Unlock()
		return
	}
	_ = l.sendTeardownPacket() // #nosec G104 - best effort notification to peer
	if l.transport != nil && len(l.linkID) > 0 {
		l.transport.UnregisterLink(l.linkID)
	}
	cb := l.closedCallback
	l.resetIncomingResource()
	l.mutex.Unlock()

	// The callback is user code and may call back into the link. It must not
	// run while l.mutex is held.
	if cb != nil {
		cb(l)
	}
}

// notifyChannelClosed wakes channel waiters blocked on this link's status.
func (l *Link) notifyChannelClosed() {
	l.channelMutex.RLock()
	ch := l.channel
	l.channelMutex.RUnlock()
	if ch != nil {
		ch.NotifyClosed()
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
func (l *Link) deliverOrQueuePlainPacket(plaintext []byte, pkt *packet.Packet) {
	l.packetCbMu.RLock()
	cb := l.packetCallback
	l.packetCbMu.RUnlock()
	if cb != nil {
		cb(plaintext, pkt)
		return
	}
	l.pendingPlainMu.Lock()
	dropped := len(l.pendingPlainData) > 0
	l.pendingPlainData = append([]byte(nil), plaintext...)
	l.pendingPlainMu.Unlock()
	if dropped {
		debug.Log(debug.DebugVerbose, common.MsgLinkNoPacketCallbackDropped)
	} else {
		debug.Log(debug.DebugVerbose, common.MsgLinkNoPacketCallback)
	}
}
func (l *Link) SetRemoteIdentifiedCallback(callback func(*Link, *identity.Identity)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.identifiedCallback = callback
}
func (l *Link) SendPacket(data []byte) error {
	return l.SendPacketWithContext(data, packet.ContextNone)
}
func (l *Link) SendPacketWithContext(data []byte, context byte) error {
	l.mutex.Lock()

	if l.status.Load() != int32(StatusActive) {
		l.mutex.Unlock()
		if debug.Enabled(debug.DebugVerbose) {
			debug.Log(debug.DebugVerbose, "Cannot send packet: link not active", "status", l.status.Load())
		}
		return common.ErrLinkNotActive
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
		CreateReceipt:   true,
		Link:            l,
	}

	var err error
	if context == packet.ContextResource || context == packet.ContextCacheReq {
		p.Data = data
		err = p.Pack()
	} else {
		err = l.sealEncryptedHT1Locked(p, data)
	}
	if err != nil {
		l.mutex.Unlock()
		if debug.Enabled(debug.DebugError) {
			debug.Log(debug.DebugError, "Failed to encrypt packet", "error", err)
		}
		return err
	}

	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Sending encrypted packet", "bytes", len(p.Data), "context", context)
	}
	l.recordOutboundData()
	l.mutex.Unlock()

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
			_ = l.promoteToActive()
		}

		l.watchdogLock = false
		l.mutex.Unlock()
		return l.handleDataPacket(pkt)
	}

	// Resource proofs prepare and advertise the next split segment which
	// encrypts under the link mutex. Unlock first like data packets so we
	// do not deadlock on a nested RLock.
	if pkt.PacketType == packet.PacketTypeProof && pkt.Context == packet.ContextResourcePRF {
		l.mutex.Lock()
		l.watchdogLock = true
		if l.status.Load() == int32(StatusClosed) {
			debug.Log(debug.DebugVerbose, "Ignoring packet for closed link", "link_id", fmt.Sprintf("%x", l.linkID))
			l.watchdogLock = false
			l.mutex.Unlock()
			return nil
		}
		l.recordInbound(true)
		if l.status.Load() == int32(StatusStale) {
			_ = l.promoteToActive()
		}
		l.watchdogLock = false
		l.mutex.Unlock()
		return l.handleResourceProof(pkt)
	}

	// LRRTT decrypts under RLock then takes Lock for RTT state. Unlock first
	// so HandleInbound does not deadlock on a nested RLock.
	if pkt.PacketType == packet.PacketTypeProof && pkt.Context == packet.ContextLRRTT {
		l.mutex.Lock()
		l.watchdogLock = true
		if l.status.Load() == int32(StatusClosed) {
			debug.Log(debug.DebugVerbose, "Ignoring packet for closed link", "link_id", fmt.Sprintf("%x", l.linkID))
			l.watchdogLock = false
			l.mutex.Unlock()
			return nil
		}
		l.recordInbound(true)
		if l.status.Load() == int32(StatusStale) {
			_ = l.promoteToActive()
		}
		l.watchdogLock = false
		l.mutex.Unlock()
		return l.handleRTTPacket(pkt)
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
		_ = l.promoteToActive()
	}

	if pkt.PacketType == packet.PacketTypeProof && pkt.Context == packet.ContextLRProof {
		return l.handleLinkProof(pkt, l.networkInterface)
	}

	return nil
}
func (l *Link) handleDataPacket(pkt *packet.Packet) error {
	st := l.status.Load()
	if st != int32(StatusActive) && st != int32(StatusHandshake) {
		return common.ErrLinkNotActive
	}

	if pkt.Context == packet.ContextLRRTT && st == int32(StatusHandshake) && !l.initiator {
		debug.Log(debug.DebugVerbose, "RTT packet detected in handleDataPacket, routing to handleRTTPacket", "link_id", fmt.Sprintf("%x", l.linkID))
		return l.handleRTTPacket(pkt)
	}

	var plaintext []byte
	var err error

	if l.hasSessionKeys() {
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
					debug.Log(debug.DebugError, "Failed to decrypt packet", "error", err, "context", fmt.Sprintf("0x%02x", pkt.Context), "link_id", fmt.Sprintf("%x", l.linkID))
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
		l.maybeProveInboundData(pkt)
	case packet.ContextRequest:
		if l.destination != nil {
			if maxSize, ok := l.destination.MaxRequestSize(); ok && len(plaintext) > maxSize {
				debug.Log(debug.DebugVerbose, "Ignored request with excessive size",
					"bytes", len(plaintext), "max", maxSize)
				return nil
			}
		}
		return l.handleRequest(plaintext, pkt.TruncatedHash())
	case packet.ContextResponse:
		return l.handleResponse(plaintext)
	case packet.ContextLinkIdentify:
		return l.HandleIdentification(plaintext)
	case packet.ContextKeepalive:
		if !l.initiator && len(plaintext) == 1 && plaintext[0] == KeepaliveRequestByte {
			// Match Python 1.4.0: only reply when outbound has been quiet
			// for a full keepalive interval.
			lastOutbound := nsToTime(l.lastOutboundNs.Load())
			if !responderShouldReplyKeepalive(time.Now(), lastOutbound, l.keepalive, l.initiator) {
				return nil
			}
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
			l.recordKeepaliveOutbound()
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
func (l *Link) handleRTTPacket(pkt *packet.Packet) error {
	if !l.initiator {
		if l.status.Load() != int32(StatusHandshake) {
			// A duplicate or replayed RTT on an established link must not
			// re-fire the established callback or reset link timing.
			return nil
		}
		measuredRTT := time.Since(l.requestTime).Seconds()
		debug.Log(debug.DebugVerbose, "Handling RTT packet (responder)", "link_id", fmt.Sprintf("%x", l.linkID), "has_session_key", l.hasSessionKeys(), "status", l.status.Load(), "data_len", len(pkt.Data))
		plaintext, err := l.decrypt(pkt.Data)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to decrypt RTT packet", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			return err
		}
		debug.Log(debug.DebugVerbose, "RTT packet decrypted successfully", "plaintext_len", len(plaintext), "link_id", fmt.Sprintf("%x", l.linkID))

		rtt, err := parseRTTPayloadSeconds(plaintext)
		if err != nil {
			debug.Log(debug.DebugError, "Failed to decode RTT payload", "error", err, "link_id", fmt.Sprintf("%x", l.linkID))
			return err
		}

		l.mutex.Lock()
		l.rtt = maxFloat(measuredRTT, rtt)
		l.establishedAt = time.Now()
		l.expectedHops = pkt.Hops
		if l.rtt > 0 {
			l.updateKeepaliveLocked()
		}
		logRtt := l.rtt
		l.mutex.Unlock()

		if !l.promoteToActive() {
			debug.Log(debug.DebugVerbose, "Ignoring late RTT on closed link", "link_id", fmt.Sprintf("%x", l.linkID))
			return nil
		}

		if l.transport != nil {
			l.transport.RegisterLink(l.linkID, l)
			if l.networkInterface != nil {
				l.registerLinkPath()
			}
		}

		if l.establishedCallback != nil {
			// Wire link DATA callbacks before returning so the first LXMF packet
			// after RTT is not queued under a nil callback and lost to overwrite.
			l.establishedCallback(l)
		}
		// Python rnsh may deliver Version before this RTT is processed. Flush
		// after handlers are registered so early channel envelopes are not dropped.
		l.flushEarlyChannel()

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
func (l *Link) handleTeardown(plaintext []byte) error {
	if len(plaintext) == len(l.linkID) && string(plaintext) == string(l.linkID) {
		if !l.closeOnce(StatusFailed) {
			return nil
		}
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

// closeOnce CAS-transitions any non-Closed status to Closed exactly once.
func (l *Link) closeOnce(reason byte) bool {
	for {
		st := l.status.Load()
		if st == int32(StatusClosed) {
			return false
		}
		if l.status.CompareAndSwap(st, int32(StatusClosed)) {
			l.teardownReason = reason
			l.dropSplitAssemblies()
			l.notifyChannelClosed()
			if l.watchdogDone != nil {
				close(l.watchdogDone)
			}
			if l.linkDone != nil {
				close(l.linkDone)
			}
			// Fail pending requests and wake SendResource waiters so teardown
			// does not leave them parked on their deadlines.
			l.failAllPendingRequests()
			l.signalOutgoingResourceComplete()
			return true
		}
	}
}

// promoteToActive CAS-transitions Handshake/Pending/Stale to Active.
// Returns false when the link is already Closed (or an unexpected state),
// preventing late RTT/proof from resurrecting a timed-out link (TOCTOU).
func (l *Link) promoteToActive() bool {
	for {
		st := l.status.Load()
		switch st {
		case int32(StatusActive):
			l.releaseOutboundEstablish()
			return true
		case int32(StatusHandshake), int32(StatusPending), int32(StatusStale):
			if l.status.CompareAndSwap(st, int32(StatusActive)) {
				l.releaseOutboundEstablish()
				return true
			}
		default:
			return false
		}
	}
}
func (l *Link) attachedIfaceName() string {
	if l == nil {
		return ""
	}
	l.mutex.RLock()
	iface := l.networkInterface
	l.mutex.RUnlock()
	if iface == nil {
		return ""
	}
	return iface.GetName()
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
	if l == nil || l.status.Load() != int32(StatusActive) {
		debug.Log(debug.DebugVerbose, common.MsgLinkNotActive)
		return nil
	}
	pkt := &packet.Packet{
		HeaderType:      packet.HeaderType1,
		PacketType:      packet.PacketTypeData,
		TransportType:   0,
		Context:         packet.ContextChannel,
		ContextFlag:     packet.FlagUnset,
		Hops:            0,
		DestinationType: DestTypeLink,
		DestinationHash: l.linkID,
		CreateReceipt:   false,
		Link:            l,
	}

	l.mutex.Lock()
	if l.status.Load() != int32(StatusActive) {
		l.mutex.Unlock()
		debug.Log(debug.DebugVerbose, common.MsgLinkNotActive)
		return nil
	}
	if err := l.sealEncryptedHT1Locked(pkt, data); err != nil {
		l.mutex.Unlock()
		return nil
	}
	l.recordOutbound()
	l.mutex.Unlock()

	if err := l.transport.SendPacket(pkt); err != nil {
		return nil
	}

	receipt := packet.NewPacketReceipt(pkt)
	receipt.SetLink(l)
	if l.transport != nil {
		l.transport.RegisterReceipt(receipt)
	}
	l.channelReceiptMu.Lock()
	if l.channelReceipts == nil {
		l.channelReceipts = make(map[*packet.Packet]*packet.PacketReceipt)
	}
	l.channelReceipts[pkt] = receipt
	l.channelReceiptMu.Unlock()

	return pkt
}
func (l *Link) SetPacketTimeout(pkt any, callback func(any), timeout time.Duration) {
	packetObj, ok := pkt.(*packet.Packet)
	if !ok || callback == nil {
		return
	}
	go func() {
		select {
		case <-l.linkDone:
			return
		case <-time.After(timeout):
		}
		l.channelReceiptMu.Lock()
		receipt := l.channelReceipts[packetObj]
		l.channelReceiptMu.Unlock()
		if receipt != nil && receipt.IsDelivered() {
			return
		}
		callback(packetObj)
	}()
}

// DropPacketReceipt stops delivery tracking for a packet whose envelope the
// channel has terminally removed. Without this, undeliverable sends leak one
// receipt per message for the life of the link.
func (l *Link) DropPacketReceipt(pkt any) {
	packetObj, ok := pkt.(*packet.Packet)
	if !ok {
		return
	}
	l.channelReceiptMu.Lock()
	delete(l.channelReceipts, packetObj)
	l.channelReceiptMu.Unlock()
}

func (l *Link) SetPacketDelivered(pkt any, callback func(any)) {
	packetObj, ok := pkt.(*packet.Packet)
	if !ok || callback == nil {
		return
	}
	l.channelReceiptMu.Lock()
	receipt := l.channelReceipts[packetObj]
	l.channelReceiptMu.Unlock()
	if receipt == nil {
		return
	}
	receipt.SetDeliveryCallback(func(*packet.PacketReceipt) {
		l.channelReceiptMu.Lock()
		delete(l.channelReceipts, packetObj)
		l.channelReceiptMu.Unlock()
		callback(packetObj)
	})
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
func (l *Link) startWatchdog() {
	if !l.watchdogActive.CompareAndSwap(false, true) {
		return
	}
	l.mutex.Lock()
	l.watchdogDone = make(chan struct{})
	l.mutex.Unlock()
	go l.watchdog()
}
func (l *Link) watchdog() {
	for l.GetStatus() != StatusClosed {
		if GlobalPaused() {
			select {
			case <-l.watchdogDone:
				l.watchdogActive.Store(false)
				return
			case <-time.After(time.Duration(WatchdogInterval * float64(time.Second))):
			}
			continue
		}
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
			select {
			case <-l.watchdogDone:
				l.watchdogActive.Store(false)
				return
			case <-time.After(time.Duration(rttWait * float64(time.Second))):
			}
			continue
		}

		var sleepTime = WatchdogInterval

		if l.status.Load() == int32(StatusPending) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				debug.Log(debug.DebugWarning, "Link establishment timed out", "link_id", fmt.Sprintf("%x", l.linkID), "status", l.status.Load())
				l.finishWatchdogClose(StatusFailed, true)
				sleepTime = 0.001
			}
		} else if l.status.Load() == int32(StatusHandshake) {
			nextCheck := l.requestTime.Add(l.establishmentTimeout)
			sleepTime = time.Until(nextCheck).Seconds()
			if time.Now().After(nextCheck) {
				elapsed := time.Since(l.requestTime).Seconds()
				if l.initiator {
					debug.Log(debug.DebugWarning, "Timeout waiting for link request proof", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				} else {
					debug.Log(debug.DebugWarning, "Timeout waiting for RTT packet from link initiator", "link_id", fmt.Sprintf("%x", l.linkID), "elapsed", fmt.Sprintf("%.3fs", elapsed), "timeout", l.establishmentTimeout.Seconds())
				}
				l.finishWatchdogClose(StatusFailed, true)
				sleepTime = 0.001
			}
		} else if l.status.Load() == int32(StatusActive) {
			activatedAt := l.establishedAt
			if activatedAt.IsZero() {
				activatedAt = time.Time{}
			}
			lastInbound := nsToTime(l.lastInboundNs.Load())
			lastOutbound := nsToTime(l.lastOutboundNs.Load())
			lastKeepalive := nsToTime(l.lastKeepaliveNs.Load())
			if lastKeepalive.IsZero() {
				lastKeepalive = lastOutbound
			}
			inboundActivity := lastInbound
			if inboundActivity.Before(activatedAt) {
				inboundActivity = activatedAt
			}
			now := time.Now()

			// Send keepalive when inbound OR outbound is older than the
			// keepalive interval so initiator probes still fire when the
			// remote side is continuously transmitting (RNS 1.4.0).
			// Throttle initiator probes on lastKeepalive so one-way local
			// data traffic does not suppress aliveness checks.
			if initiatorShouldSendKeepalive(now, inboundActivity, lastOutbound, lastKeepalive, l.keepalive, l.initiator) {
				_ = l.sendKeepalive() // #nosec G104 - best effort keepalive
			}
			needKeepalive := keepaliveDue(now, inboundActivity, lastOutbound, l.keepalive)
			if needKeepalive {
				if now.After(inboundActivity.Add(l.staleTime)) {
					sleepTime = l.rtt*KeepaliveTimeoutFactor + StaleGrace
					if l.status.CompareAndSwap(int32(StatusActive), int32(StatusStale)) {
						ifaceName := ""
						if l.networkInterface != nil {
							ifaceName = l.networkInterface.GetName()
						}
						health.Inc(ifaceName, health.KindKeepaliveTimeout)
					} else if l.status.Load() != int32(StatusStale) {
						sleepTime = float64(l.keepalive) / float64(time.Second)
					}
				} else {
					sleepTime = float64(l.keepalive) / float64(time.Second)
				}
			} else {
				nextIn := inboundActivity.Add(l.keepalive)
				nextOut := lastOutbound.Add(l.keepalive)
				nextKeepalive := nextIn
				if nextOut.Before(nextKeepalive) {
					nextKeepalive = nextOut
				}
				sleepTime = time.Until(nextKeepalive).Seconds()
			}
		} else if l.status.Load() == int32(StatusStale) {
			sleepTime = 0.001
			debug.Log(debug.DebugWarning, "Link marked stale, closing", "link_id", fmt.Sprintf("%x", l.linkID))
			ifaceName := ""
			if l.networkInterface != nil {
				ifaceName = l.networkInterface.GetName()
			}
			health.Inc(ifaceName, health.KindLinkStaleClose)
			_ = l.sendTeardownPacket() // #nosec G104 - best effort teardown
			l.finishWatchdogClose(StatusFailed, false)
			sleepTime = 0.001
		}

		if sleepTime <= 0.0 {
			sleepTime = 0.1
		}
		if sleepTime > 5.0 {
			sleepTime = 5.0
		}

		l.mutex.Unlock()
		select {
		case <-l.watchdogDone:
			l.watchdogActive.Store(false)
			return
		case <-time.After(time.Duration(sleepTime * float64(time.Second))):
		}
	}
	l.watchdogActive.Store(false)
}

// finishWatchdogClose CAS-closes the link once and runs close side effects.
// invalidatePath applies initiator path invalidation for establishment timeouts.
func (l *Link) finishWatchdogClose(reason byte, invalidatePath bool) {
	if !l.closeOnce(reason) {
		return
	}
	if l.transport != nil && len(l.linkID) > 0 {
		l.transport.UnregisterLink(l.linkID)
	}
	l.releaseOutboundEstablish()
	if invalidatePath && l.initiator {
		l.invalidateTransportPathAfterInitiatorFailure()
	}
	// Callers hold l.mutex. User callbacks may call back into the link.
	if l.closedCallback != nil {
		cb := l.closedCallback
		go cb(l)
	}
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
	encrypted, err := l.encryptLocked(l.linkID)
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

// tryTerminusPathRebalanceLocked applies RNS 1.4.1 initiator-side hop correction
// when a signed LRPROOF arrives with a different hop count while PENDING.
// Link mutex must be held. Returns true when expectedHops now matches accounted.
func (l *Link) tryTerminusPathRebalanceLocked(pkt *packet.Packet, networkIface common.NetworkInterface, accounted uint8) bool {
	if l == nil || pkt == nil || l.transport == nil {
		return false
	}
	if l.status.Load() != int32(StatusPending) {
		return false
	}
	if !l.rebalanced.IsZero() {
		return false
	}
	if !l.transport.LinkPathRebalanceAllowed() {
		return false
	}
	if len(pkt.Data) < identity.SigLength/8+KeySize {
		return false
	}
	signature := pkt.Data[0 : identity.SigLength/8]
	peerPub := pkt.Data[identity.SigLength/8 : identity.SigLength/8+KeySize]
	signalling := []byte{0, 0, 0}
	if len(pkt.Data) >= identity.SigLength/8+KeySize+LinkMTUSize {
		signalling = pkt.Data[identity.SigLength/8+KeySize : identity.SigLength/8+KeySize+LinkMTUSize]
		mode := (signalling[0] & ModeByteMask) >> 5
		if l.mode != 0 && mode != l.mode {
			debug.Log(debug.DebugVerbose, "Aborting terminus path rebalance due to link mode mismatch",
				"got", mode, "want", l.mode)
			return false
		}
	}
	peerSigPub := l.peerSigPub
	if len(peerSigPub) == 0 && l.destination != nil && l.destination.GetIdentity() != nil {
		pubKey := l.destination.GetIdentity().GetPublicKey()
		if len(pubKey) >= ECPubSize {
			peerSigPub = pubKey[KeySize:ECPubSize]
		}
	}
	if len(peerSigPub) == 0 || l.destination == nil || l.destination.GetIdentity() == nil {
		return false
	}
	signedData := make([]byte, 0, len(l.linkID)+KeySize+len(peerSigPub)+len(signalling))
	signedData = append(signedData, l.linkID...)
	signedData = append(signedData, peerPub...)
	signedData = append(signedData, peerSigPub...)
	signedData = append(signedData, signalling...)
	if !l.destination.GetIdentity().Verify(signedData, signature) {
		debug.Log(debug.DebugVerbose, "Aborting terminus path rebalance due to invalid signature",
			"link_id", fmt.Sprintf("%x", l.linkID))
		return false
	}
	destHash := l.destination.GetHash()
	if !l.transport.RebalancePathHops(destHash, accounted, networkIface) {
		return false
	}
	l.rebalanced = time.Now()
	l.expectedHops = accounted
	debug.Log(debug.DebugVerbose, "Re-balancing path at link terminus",
		"link_id", fmt.Sprintf("%x", l.linkID),
		"to", accounted)
	return accounted == l.expectedHops
}
