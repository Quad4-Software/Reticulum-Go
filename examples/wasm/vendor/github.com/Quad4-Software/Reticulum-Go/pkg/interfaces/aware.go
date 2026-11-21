// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// WiFi Aware (NAN) interface, Android only in practice.
//
// The platform driver owns the NAN session, publish/subscribe, data-path
// negotiation, and the per-peer sockets. The controller here mirrors the
// Python AwareInterface: each link_up event spawns a per-peer interface that
// carries HDLC-framed packets, identical on the wire to TCPClientInterface.
//
// Config example:
//
//	[[WiFi Aware]]
//	  type = AwareInterface
//	  enabled = yes
//	  role = subscribe
//	  peers = 4
//
// role is publish (responder, waits for initiators) or subscribe
// (initiator, connects to discovered publishers). peers caps concurrent
// data paths.

const (
	// AwareBitrateGuess matches the Python interface bitrate hint.
	AwareBitrateGuess = 10_000_000

	// AwareMaxPeersLimit is the hard cap on concurrent data paths.
	AwareMaxPeersLimit = 8

	// AwareDefaultPeers is the default concurrent data-path cap.
	AwareDefaultPeers = 4
)

// AwareDriver is the host-platform NAN session. Android bridges implement
// this over gomobile; other platforms may return errors on Start.
type AwareDriver interface {
	StartPublish() error
	StartSubscribe() error
	// Send writes framed bytes to one peer's data-path socket.
	Send(peerID int, data []byte) error
	// ClosePeer tears down one data path.
	ClosePeer(peerID int) error
	// Stop ends the NAN session and all data paths.
	Stop() error
	// SetEvents installs the controller's event callbacks.
	SetEvents(AwareEvents)
}

// AwareEvents delivers driver lifecycle events to the controller. All are
// invoked from driver threads; implementations must be goroutine-safe.
type AwareEvents struct {
	// OnLinkUp reports a data-path socket ready for framing.
	OnLinkUp func(peerID int)
	// OnData delivers raw bytes received on a peer socket.
	OnData func(peerID int, data []byte)
	// OnLinkDown reports a peer socket closed or data path lost.
	OnLinkDown func(peerID int)
	// OnSessionLost reports the whole Aware session died.
	OnSessionLost func()
}

// AwareSpawnHook registers a spawned peer interface with the transport,
// matching the SpawnLocal hook for shared-instance clients.
type AwareSpawnHook func(peer *AwarePeerInterface)

// AwareInterface is the controller: it owns the platform NAN session and
// spawns one AwarePeerInterface per established data path. The controller
// itself carries no packets.
type AwareInterface struct {
	BaseInterface
	Mutex       sync.RWMutex
	driver      AwareDriver
	spawn       AwareSpawnHook
	role        string
	maxPeers    int
	done        chan struct{}
	stopOnce    sync.Once
	unregister  func(name string)
	peers       map[int]*AwarePeerInterface
	spawning    map[int]struct{}
	peerKey     string
}

// NewAwareInterface builds a controller around a host-supplied driver. A nil
// driver fails at Start, same as the Python bridge raising SystemError on
// non-Android platforms.
func NewAwareInterface(name, role string, peers int, driver AwareDriver, spawn AwareSpawnHook) (*AwareInterface, error) {
	if role == "" {
		role = "subscribe"
	}
	if role != "publish" && role != "subscribe" {
		return nil, fmt.Errorf("invalid aware role %q (want publish or subscribe)", role)
	}
	if peers < 1 {
		peers = AwareDefaultPeers
	}
	if peers > AwareMaxPeersLimit {
		peers = AwareMaxPeersLimit
	}
	ai := &AwareInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeTCP, true),
		driver:        driver,
		spawn:         spawn,
		role:          role,
		maxPeers:      peers,
		done:          make(chan struct{}),
		peers:         make(map[int]*AwarePeerInterface),
		spawning:      make(map[int]struct{}),
	}
	ai.In = true
	ai.Out = false
	ai.Bitrate = AwareBitrateGuess
	ai.MTU = common.DefaultMTU
	return ai, nil
}

// SetUnregisterHook installs the transport-side removal callback invoked
// when a peer tears itself down.
func (ai *AwareInterface) SetUnregisterHook(fn func(name string)) {
	ai.unregister = fn
}

// Start activates the NAN session in the configured role.
func (ai *AwareInterface) Start() error {
	if ai.driver == nil {
		return errors.New("AwareInterface requires a host aware driver (Android local-link bridge)")
	}
	ai.Mutex.Lock()
	select {
	case <-ai.done:
		ai.done = make(chan struct{})
		ai.stopOnce = sync.Once{}
	default:
	}
	ai.Mutex.Unlock()

	ai.driver.SetEvents(AwareEvents{
		OnLinkUp:      ai.onLinkUp,
		OnData:        ai.onData,
		OnLinkDown:    ai.onLinkDown,
		OnSessionLost: ai.onSessionLost,
	})

	var err error
	if ai.role == "publish" {
		err = ai.driver.StartPublish()
	} else {
		err = ai.driver.StartSubscribe()
	}
	if err != nil {
		return fmt.Errorf("aware session start failed: %w", err)
	}
	ai.Mutex.Lock()
	ai.Online = true
	ai.Mutex.Unlock()
	debug.Log(debug.DebugVerbose, "Aware session started", "name", ai.Name, "role", ai.role)
	return nil
}

// Stop ends the session and tears down all spawned peers.
func (ai *AwareInterface) Stop() error {
	ai.Mutex.Lock()
	ai.Online = false
	peers := make([]*AwarePeerInterface, 0, len(ai.peers))
	for _, p := range ai.peers {
		peers = append(peers, p)
	}
	ai.peers = make(map[int]*AwarePeerInterface)
	ai.Mutex.Unlock()
	ai.stopOnce.Do(func() {
		if ai.done != nil {
			close(ai.done)
		}
	})
	for _, p := range peers {
		p.teardownLocked()
	}
	if ai.driver != nil {
		return ai.driver.Stop()
	}
	return nil
}

func (ai *AwareInterface) isStopped() bool {
	select {
	case <-ai.done:
		return true
	default:
		return false
	}
}

// onLinkUp spawns a peer interface for a fresh data path, honoring the
// concurrent-peer cap like the Python spawn path.
func (ai *AwareInterface) onLinkUp(peerID int) {
	if ai.isStopped() {
		return
	}
	ai.Mutex.Lock()
	if _, ok := ai.peers[peerID]; ok {
		ai.Mutex.Unlock()
		return
	}
	if _, ok := ai.spawning[peerID]; ok {
		ai.Mutex.Unlock()
		return
	}
	if len(ai.peers)+len(ai.spawning) >= ai.maxPeers {
		ai.Mutex.Unlock()
		_ = ai.driver.ClosePeer(peerID)
		debug.Log(debug.DebugVerbose, "Aware peer cap reached, dropping link", "name", ai.Name, "peer", peerID)
		return
	}
	ai.spawning[peerID] = struct{}{}
	ai.Mutex.Unlock()

	defer func() {
		ai.Mutex.Lock()
		delete(ai.spawning, peerID)
		ai.Mutex.Unlock()
	}()

	peer := newAwarePeerInterface(ai, peerID)

	// Propagate controller settings the way the Python spawn path does.
	ai.Mutex.RLock()
	peer.Mode = ai.Mode
	peer.Bitrate = ai.Bitrate
	peer.MTU = ai.MTU
	peer.Gravity = ai.Gravity
	ai.Mutex.RUnlock()

	ai.Mutex.Lock()
	// Re-check cap and stopped state: both can change during construction.
	if ai.isStopped() || len(ai.peers) >= ai.maxPeers {
		ai.Mutex.Unlock()
		_ = ai.driver.ClosePeer(peerID)
		return
	}
	ai.peers[peerID] = peer
	ai.Mutex.Unlock()

	if ai.spawn != nil {
		ai.spawn(peer)
	}
	debug.Log(debug.DebugVerbose, "Spawned aware peer interface", "name", peer.Name, "peer", peerID)
}

func (ai *AwareInterface) onData(peerID int, data []byte) {
	ai.Mutex.RLock()
	peer := ai.peers[peerID]
	ai.Mutex.RUnlock()
	if peer != nil {
		peer.feed(data)
	}
}

func (ai *AwareInterface) onLinkDown(peerID int) {
	ai.Mutex.Lock()
	peer := ai.peers[peerID]
	delete(ai.peers, peerID)
	ai.Mutex.Unlock()
	if peer != nil {
		peer.teardownLocked()
	}
}

func (ai *AwareInterface) onSessionLost() {
	_ = ai.Stop()
}

// removePeer drops a peer that tore itself down.
func (ai *AwareInterface) removePeer(peerID int) {
	ai.Mutex.Lock()
	delete(ai.peers, peerID)
	ai.Mutex.Unlock()
}

// PeerCount reports live spawned peers for status surfaces.
func (ai *AwareInterface) PeerCount() int {
	ai.Mutex.RLock()
	defer ai.Mutex.RUnlock()
	return len(ai.peers)
}

func (ai *AwareInterface) String() string {
	return fmt.Sprintf("AwareInterface[%s]", ai.Name)
}

// AwarePeerInterface is one spawned peer wrapping an Aware data-path socket.
type AwarePeerInterface struct {
	BaseInterface
	parent   *AwareInterface
	peerID   int
	decoder  *hdlcStreamDecoder
	done     chan struct{}
	stopOnce sync.Once
	txMu     sync.Mutex
	txFrame  []byte
	closed   bool
}

func newAwarePeerInterface(parent *AwareInterface, peerID int) *AwarePeerInterface {
	p := &AwarePeerInterface{
		BaseInterface: NewBaseInterface(
			fmt.Sprintf("aware-peer-%d", peerID),
			common.IFTypeTCP,
			true,
		),
		parent: parent,
		peerID: peerID,
		done:   make(chan struct{}),
	}
	p.In = true
	p.Out = true
	p.Online = true
	p.decoder = newTCPHDLCStreamDecoder(p.MTU, p.handlePacket)
	return p
}

// PeerID exposes the driver-assigned peer handle.
func (p *AwarePeerInterface) PeerID() int {
	return p.peerID
}

func (p *AwarePeerInterface) handlePacket(frame []byte) {
	p.ProcessIncomingFrom(frame, fmt.Sprintf("aware-%d", p.peerID))
}

// ProcessOutgoing HDLC-frames the packet onto the data-path socket, exactly
// like TCPClientInterface framing.
func (p *AwarePeerInterface) ProcessOutgoing(data []byte) error {
	if err := common.RejectReceiveOnly(p); err != nil {
		return err
	}
	p.Mutex.RLock()
	online := p.Online && !p.Detached
	p.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("aware peer offline")
	}
	p.txMu.Lock()
	frame := appendFrameHDLC(p.txFrame[:0], data)
	p.txMu.Unlock()
	if err := p.parent.driver.Send(p.peerID, frame); err != nil {
		debug.Log(debug.DebugError, "Aware send failed, tearing down", "name", p.Name, "error", err)
		p.teardownLocked()
		return err
	}
	p.Mutex.Lock()
	p.TxBytes += uint64(len(frame))
	p.TxPackets++
	p.Mutex.Unlock()
	if p.parent != nil {
		p.parent.Mutex.Lock()
		p.parent.TxBytes += uint64(len(frame))
		p.parent.TxPackets++
		p.parent.Mutex.Unlock()
	}
	return nil
}

// feed deframes raw socket bytes from the driver into packets.
func (p *AwarePeerInterface) feed(data []byte) {
	select {
	case <-p.done:
		return
	default:
	}
	if len(data) == 0 {
		return
	}
	p.decoder.feed(data)
}

// teardownLocked closes the data path and detaches from the parent.
func (p *AwarePeerInterface) teardownLocked() {
	p.stopOnce.Do(func() {
		p.Mutex.Lock()
		p.Online = false
		p.Detached = true
		p.closed = true
		p.Mutex.Unlock()
		close(p.done)
	})
	if p.parent != nil {
		if p.parent.driver != nil {
			_ = p.parent.driver.ClosePeer(p.peerID)
		}
		p.parent.removePeer(p.peerID)
		if p.parent.unregister != nil {
			p.parent.unregister(p.Name)
		}
	}
}

// Stop detaches the peer.
func (p *AwarePeerInterface) Stop() error {
	p.teardownLocked()
	return nil
}

func (p *AwarePeerInterface) String() string {
	return fmt.Sprintf("AwarePeerInterface[%s]", p.Name)
}
