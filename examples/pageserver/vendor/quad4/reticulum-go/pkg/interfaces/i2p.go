// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/i2p"
)

const (
	i2pReconnectWait   = 15 * time.Second
	i2pReadTimeoutSec  = (I2PProbeIntervalSec*I2PProbesCount + I2PProbeAfterSec) * 2
	i2pBitrateGuess    = 256 * 1000
	i2pServerRetryWait = 15 * time.Second
	i2pServerRetryMax  = 60 * time.Second
	i2pDialTimeout     = 2 * time.Minute
)

const (
	i2pTunnelStateInit   = 0x00
	i2pTunnelStateActive = 0x01
	i2pTunnelStateStale  = 0x02
)

// FromConfigContext carries runtime dependencies for interface types that
// need storage paths, transport identity, or dynamic peer registration.
type FromConfigContext struct {
	I2PStoragePath        string
	TransportID           []byte
	RegisterPeer          func(name string, peer common.NetworkInterface) error
	UnregisterPeer        func(name string)
	SetupPeer             func(peer common.NetworkInterface)
	SynthesizeTunnel      func(TunnelPeer)
	VoidTunnel            func(TunnelPeer)
	WatchInterfaces       bool
	DiscoverInterfaces    bool
	PanicOnInterfaceError bool
	BackboneHub           *backbone.Hub
	SpawnBackbone         func(client *BackboneClientInterface)
	SpawnLocal            LocalSpawnHook
	// ConfigDir is the directory containing config and the interfaces/ plugin tree.
	ConfigDir string
}

// I2PInterface is the parent listener for inbound I2P peers and optional SAM
// server tunnel publication.
type I2PInterface struct {
	BaseInterface
	controller        *i2p.Controller
	connectable       bool
	samAddress        string
	b32               string
	bindPort          int
	listener          net.Listener
	spawned           []*I2PInterfacePeer
	spawnMu           sync.Mutex
	ctx               *FromConfigContext
	cfg               *common.InterfaceConfig
	transportID       []byte
	supportsDiscovery bool
	serverDone        chan struct{}
	serverStop        sync.Once
	acceptDone        chan struct{}
	acceptStop        sync.Once
}

// I2PInterfacePeer is a logical Reticulum interface over one I2P stream.
type I2PInterfacePeer struct {
	BaseInterface
	parent            *I2PInterface
	conn              net.Conn
	session           *i2p.Session
	targetDest        string
	initiator         bool
	parentCount       bool
	reconnecting      bool
	neverConnected    bool
	awaitingTunnel    bool
	kissFraming       bool
	wantsTunnel       bool
	tunnelID          []byte
	maxReconnectTries int
	sendMu            sync.Mutex
	lastRead          time.Time
	lastWrite         time.Time
	lastError         string
	tunnelState       atomic.Uint32
	wdReset           atomic.Bool
	done              chan struct{}
	stopOnce          sync.Once
}

func NewI2PInterface(name string, cfg *common.InterfaceConfig, ctx *FromConfigContext) (*I2PInterface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil interface config")
	}
	storage := ""
	sam := cfg.I2PSAMAddress
	var transportID []byte
	if ctx != nil {
		storage = ctx.I2PStoragePath
		transportID = append([]byte(nil), ctx.TransportID...)
	}
	if storage != "" {
		storage = storage + "/i2p"
	}
	ctrl := i2p.NewController(storage, sam)
	port, err := ctrl.FreePort()
	if err != nil {
		return nil, err
	}
	parent := &I2PInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeI2P, cfg.Enabled),
		controller:        ctrl,
		connectable:       cfg.I2PConnectable,
		samAddress:        sam,
		bindPort:          port,
		ctx:               ctx,
		cfg:               cfg,
		transportID:       transportID,
		supportsDiscovery: true,
		serverDone:        make(chan struct{}),
		acceptDone:        make(chan struct{}),
	}
	parent.In = true
	parent.Out = false
	parent.MTU = DefaultMTU
	parent.Bitrate = i2pBitrateGuess
	parent.Mode = common.IFModeFull
	applyI2PParentConfig(parent, cfg)
	return parent, nil
}

func (p *I2PInterface) Start() error {
	p.Mutex.Lock()
	if !p.Enabled || p.Detached {
		p.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if p.listener != nil {
		p.Online = true
		p.Mutex.Unlock()
		return nil
	}
	p.Mutex.Unlock()

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.bindPort)))
	if err != nil {
		return common.WrapListenError(err)
	}
	p.Mutex.Lock()
	p.listener = ln
	p.Online = true
	p.Mutex.Unlock()

	go p.acceptLoop()

	if p.connectable {
		go p.serverTunnelLoop()
	}

	return nil
}

func (p *I2PInterface) Stop() error {
	p.serverStop.Do(func() { close(p.serverDone) })
	p.acceptStop.Do(func() { close(p.acceptDone) })
	p.Mutex.Lock()
	if p.listener != nil {
		_ = p.listener.Close()
		p.listener = nil
	}
	p.Online = false
	p.Mutex.Unlock()
	p.controller.Stop()
	p.spawnMu.Lock()
	peers := append([]*I2PInterfacePeer(nil), p.spawned...)
	p.spawnMu.Unlock()
	for _, peer := range peers {
		if p.ctx != nil && p.ctx.UnregisterPeer != nil {
			p.ctx.UnregisterPeer(peer.GetName())
		}
		_ = peer.Stop()
	}
	p.spawnMu.Lock()
	p.spawned = nil
	p.spawnMu.Unlock()
	return nil
}

func (p *I2PInterface) Detach() {
	debug.Log(debug.DebugInfo, "Detaching I2P interface", "name", p.Name)
	_ = p.Stop()
	p.BaseInterface.Detach()
}

func (p *I2PInterface) ProcessOutgoing([]byte) error {
	return nil
}

func (p *I2PInterface) Clients() int {
	p.spawnMu.Lock()
	defer p.spawnMu.Unlock()
	return len(p.spawned)
}

func (p *I2PInterface) LocalAddr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(p.bindPort))
}

func (p *I2PInterface) Base32() string {
	return p.b32
}

func (p *I2PInterface) Connectable() bool {
	return p.connectable
}

func (p *I2PInterface) SupportsDiscovery() bool {
	return p.supportsDiscovery
}

func (p *I2PInterface) InterfaceConfig() *common.InterfaceConfig {
	return p.cfg
}

func (p *I2PInterface) acceptLoop() {
	for {
		select {
		case <-p.acceptDone:
			return
		default:
		}
		p.Mutex.RLock()
		ln := p.listener
		p.Mutex.RUnlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-p.acceptDone:
				return
			default:
				time.Sleep(200 * time.Millisecond)
				continue
			}
		}
		peerName := "Connected peer on " + p.Name
		peer := newI2PInterfacePeerAccepted(p, peerName, conn)
		p.registerSpawnedPeer(peer)
		go peer.readLoop()
	}
}

func (p *I2PInterface) serverTunnelLoop() {
	backoff := i2pServerRetryWait
	for {
		select {
		case <-p.serverDone:
			return
		default:
		}
		for len(p.transportID) == 0 {
			select {
			case <-p.serverDone:
				return
			case <-time.After(time.Second):
			}
		}
		_, dest, err := p.controller.StartServerTunnel(p.Name, p.transportID, p.bindPort)
		if err != nil {
			debug.Log(debug.DebugError, "I2P server tunnel setup failed", "name", p.Name, "error", err)
			p.Mutex.Lock()
			p.Online = false
			p.Mutex.Unlock()
			select {
			case <-p.serverDone:
				return
			case <-time.After(backoff):
			}
			if backoff < i2pServerRetryMax {
				backoff *= 2
				if backoff > i2pServerRetryMax {
					backoff = i2pServerRetryMax
				}
			}
			continue
		}
		backoff = i2pServerRetryWait
		if dest != nil {
			p.b32 = dest.Base32()
			p.Mutex.Lock()
			p.Online = true
			p.Mutex.Unlock()
		}
		// Park until shutdown. Accept loop inside the server tunnel
		// retries transient SAM accept failures on its own.
		select {
		case <-p.serverDone:
			return
		}
	}
}

func (p *I2PInterface) registerSpawnedPeer(peer *I2PInterfacePeer) {
	if p.cfg != nil {
		if err := ApplyIFACFromConfig(peer, p.cfg); err != nil {
			debug.Log(debug.DebugError, "Failed to apply IFAC to I2P peer", "name", peer.GetName(), "error", err)
		}
	}
	p.spawnMu.Lock()
	p.spawned = append(p.spawned, peer)
	p.spawnMu.Unlock()
	if p.ctx != nil && p.ctx.RegisterPeer != nil {
		if err := p.ctx.RegisterPeer(peer.GetName(), peer); err != nil {
			debug.Log(debug.DebugError, "Failed to register I2P peer", "name", peer.GetName(), "error", err)
			return
		}
	}
	if p.ctx != nil && p.ctx.SetupPeer != nil {
		p.ctx.SetupPeer(peer)
	}
	if err := peer.Start(); err != nil {
		debug.Log(debug.DebugError, "Failed to start I2P peer", "name", peer.GetName(), "error", err)
	}
}

func NewI2PInterfacePeer(parent *I2PInterface, name, targetDest string, maxReconnect int, cfg *common.InterfaceConfig) *I2PInterfacePeer {
	if maxReconnect == 0 {
		maxReconnect = -1
	}
	peer := &I2PInterfacePeer{
		BaseInterface:     NewBaseInterface(name, common.IFTypeI2P, parent.Enabled),
		parent:            parent,
		targetDest:        targetDest,
		initiator:         true,
		parentCount:       false,
		awaitingTunnel:    true,
		neverConnected:    true,
		maxReconnectTries: maxReconnect,
		done:              make(chan struct{}),
	}
	peer.In = true
	peer.Out = true
	peer.MTU = DefaultMTU
	peer.Bitrate = i2pBitrateGuess
	peer.Mode = common.IFModeFull
	applyI2PPeerConfig(peer, cfg)
	go peer.tunnelSetupLoop()
	return peer
}

func newI2PInterfacePeerAccepted(parent *I2PInterface, name string, conn net.Conn) *I2PInterfacePeer {
	peer := &I2PInterfacePeer{
		BaseInterface: NewBaseInterface(name, common.IFTypeI2P, parent.Enabled),
		parent:        parent,
		conn:          conn,
		initiator:     false,
		parentCount:   true,
		done:          make(chan struct{}),
	}
	peer.In = true
	peer.Out = true
	peer.MTU = parent.MTU
	peer.Bitrate = parent.Bitrate
	peer.Mode = common.IFModeFull
	applyI2PPeerConfig(peer, parent.cfg)
	peer.Online = true
	_ = setI2PConnTimeouts(conn)
	return peer
}

func (peer *I2PInterfacePeer) Start() error {
	peer.Mutex.Lock()
	defer peer.Mutex.Unlock()
	if peer.initiator && peer.conn == nil {
		return nil
	}
	peer.Online = true
	return nil
}

func (peer *I2PInterfacePeer) Stop() error {
	peer.stopOnce.Do(func() {
		close(peer.done)
	})
	peer.closeStream()
	return nil
}

func (peer *I2PInterfacePeer) Detach() {
	debug.Log(debug.DebugInfo, "Detaching I2P peer", "name", peer.Name)
	peer.BaseInterface.Detach()
	_ = peer.Stop()
}

func (peer *I2PInterfacePeer) GetConn() net.Conn {
	peer.Mutex.RLock()
	defer peer.Mutex.RUnlock()
	return peer.conn
}

func (peer *I2PInterfacePeer) TunnelState() uint32 {
	return peer.tunnelState.Load()
}

// LastError returns the most recent SAM dial or stream error text.
func (peer *I2PInterfacePeer) LastError() string {
	peer.Mutex.RLock()
	defer peer.Mutex.RUnlock()
	return peer.lastError
}

func (peer *I2PInterfacePeer) String() string {
	return "I2PInterfacePeer[" + peer.Name + "]"
}

func (peer *I2PInterfacePeer) InterfaceHash() []byte {
	return InterfaceHashFromName(peer.Name)
}

func (peer *I2PInterfacePeer) WantsTunnel() bool {
	return peer.wantsTunnel
}

func (peer *I2PInterfacePeer) SetWantsTunnel(v bool) {
	peer.wantsTunnel = v
}

func (peer *I2PInterfacePeer) TunnelID() []byte {
	peer.Mutex.RLock()
	defer peer.Mutex.RUnlock()
	return append([]byte(nil), peer.tunnelID...)
}

func (peer *I2PInterfacePeer) SetTunnelID(id []byte) {
	peer.Mutex.Lock()
	defer peer.Mutex.Unlock()
	peer.tunnelID = append(peer.tunnelID[:0], id...)
}

func (peer *I2PInterfacePeer) InterfaceConfig() *common.InterfaceConfig {
	if peer.parent == nil {
		return nil
	}
	return peer.parent.cfg
}

func (peer *I2PInterfacePeer) onConnected() {
	if peer.wantsTunnel && peer.parent != nil && peer.parent.ctx != nil && peer.parent.ctx.SynthesizeTunnel != nil {
		peer.parent.ctx.SynthesizeTunnel(peer)
	}
}

func (peer *I2PInterfacePeer) closeStream() {
	peer.Mutex.Lock()
	conn := peer.conn
	sess := peer.session
	peer.conn = nil
	peer.session = nil
	peer.Online = false
	peer.Mutex.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if peer.parent != nil && peer.parent.controller != nil && sess != nil {
		peer.parent.controller.ReleaseDialSession(sess)
	} else if sess != nil {
		_ = sess.Close()
	}
}

func (peer *I2PInterfacePeer) tunnelSetupLoop() {
	peer.awaitingTunnel = true
	peer.tunnelState.Store(i2pTunnelStateInit)
	if !peer.dialStream(true) {
		go peer.reconnect()
		return
	}
	go peer.readLoop()
}

// dialStream opens a direct SAM STREAM to the peer destination.
// Online is set only after STREAM CONNECT succeeds.
func (peer *I2PInterfacePeer) dialStream(initial bool) bool {
	select {
	case <-peer.done:
		return false
	default:
	}
	if peer.parent == nil || peer.parent.controller == nil {
		return false
	}
	peer.closeStream()
	peer.tunnelState.Store(i2pTunnelStateInit)
	parentCtx := peer.parent.controller.Ctx()
	ctx, cancel := context.WithTimeout(parentCtx, i2pDialTimeout)
	defer cancel()
	go func() {
		select {
		case <-peer.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	conn, sess, err := peer.parent.controller.DialStream(ctx, peer.targetDest)
	if err != nil {
		peer.Mutex.Lock()
		peer.lastError = err.Error()
		peer.Online = false
		peer.Mutex.Unlock()
		if initial {
			debug.Log(debug.DebugError, "I2P peer stream dial failed", "name", peer.Name, "error", err)
		}
		return false
	}
	_ = setI2PConnTimeouts(conn)
	peer.Mutex.Lock()
	peer.conn = conn
	peer.session = sess
	peer.Online = true
	peer.neverConnected = false
	peer.awaitingTunnel = false
	peer.lastError = ""
	peer.Mutex.Unlock()
	peer.tunnelState.Store(i2pTunnelStateActive)
	peer.onConnected()
	return true
}

func (peer *I2PInterfacePeer) reconnect() {
	peer.Mutex.Lock()
	if peer.reconnecting || !peer.initiator {
		peer.Mutex.Unlock()
		return
	}
	peer.reconnecting = true
	peer.Mutex.Unlock()
	defer func() {
		peer.Mutex.Lock()
		peer.reconnecting = false
		peer.Mutex.Unlock()
	}()

	peer.closeStream()
	peer.tunnelState.Store(i2pTunnelStateInit)

	attempts := 0
	for {
		select {
		case <-peer.done:
			return
		default:
		}
		peer.Mutex.RLock()
		online := peer.Online && peer.conn != nil
		detached := peer.Detached
		peer.Mutex.RUnlock()
		if online || detached {
			break
		}
		time.Sleep(i2pReconnectWait)
		attempts++
		if peer.maxReconnectTries >= 0 && attempts > peer.maxReconnectTries {
			debug.Log(debug.DebugError, "I2P peer max reconnect attempts reached", "name", peer.Name)
			peer.teardown()
			return
		}
		if peer.dialStream(false) {
			break
		}
	}
	if !peer.neverConnected {
		debug.Log(debug.DebugInfo, "I2P peer re-established connection", "name", peer.Name)
		peer.wantsTunnel = !peer.kissFraming
	}
	go peer.readLoop()
}

func (peer *I2PInterfacePeer) ProcessOutgoing(data []byte) error {
	peer.Mutex.RLock()
	conn := peer.conn
	online := peer.Online
	peer.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("interface offline")
	}
	masked, err := common.ApplyIFACOutbound(peer, data)
	if err != nil {
		return err
	}
	data = masked

	peer.sendMu.Lock()
	defer peer.sendMu.Unlock()

	var frame []byte
	if peer.kissFraming {
		frame = appendFrameKISS(nil, data)
	} else {
		frame = append([]byte{HDLCFlag}, escapeHDLC(data)...)
		frame = append(frame, HDLCFlag)
	}
	_, err = conn.Write(frame)
	if err == nil {
		peer.Mutex.Lock()
		peer.lastWrite = time.Now()
		peer.TxBytes += uint64(len(frame))
		peer.TxPackets++
		peer.Mutex.Unlock()
		if peer.parentCount && peer.parent != nil {
			peer.parent.Mutex.Lock()
			peer.parent.TxBytes += uint64(len(frame))
			peer.parent.TxPackets++
			peer.parent.Mutex.Unlock()
		}
	} else {
		debug.Log(debug.DebugError, "I2P peer transmit failed", "name", peer.Name, "error", err)
		peer.teardown()
	}
	return err
}

func (peer *I2PInterfacePeer) readLoop() {
	go peer.readWatchdog()
	peer.Mutex.Lock()
	peer.lastRead = time.Now()
	peer.lastWrite = time.Now()
	peer.Mutex.Unlock()

	var feed func([]byte)
	if peer.kissFraming {
		decoder := newKISSStreamDecoder(peer.MTU, peer.deliverFrame)
		feed = decoder.feed
	} else {
		decoder := newHDLCToggleStreamDecoder(peer.MTU, peer.deliverFrame)
		feed = decoder.feed
	}

	for {
		select {
		case <-peer.done:
			return
		default:
		}
		peer.Mutex.RLock()
		conn := peer.conn
		peer.Mutex.RUnlock()
		if conn == nil {
			return
		}
		buf := make([]byte, peer.MTU)
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			peer.Mutex.Lock()
			peer.Online = false
			initiator := peer.initiator
			detached := peer.Detached
			peer.Mutex.Unlock()
			peer.tunnelState.Store(i2pTunnelStateInit)
			peer.wdReset.Store(true)
			time.Sleep(2 * time.Second)
			peer.wdReset.Store(false)
			if initiator && !detached {
				go peer.reconnect()
			} else {
				peer.teardown()
			}
			return
		}
		peer.Mutex.Lock()
		peer.lastRead = time.Now()
		peer.Mutex.Unlock()
		feed(buf[:n])
	}
}

func (peer *I2PInterfacePeer) deliverFrame(data []byte) {
	if len(data) == 0 {
		return
	}
	peer.Mutex.Lock()
	peer.RxBytes += uint64(len(data))
	peer.RxPackets++
	peer.Mutex.Unlock()
	if peer.parentCount && peer.parent != nil {
		peer.parent.Mutex.Lock()
		peer.parent.RxBytes += uint64(len(data))
		peer.parent.RxPackets++
		peer.parent.Mutex.Unlock()
	}
	peer.ProcessIncoming(data)
}

func (peer *I2PInterfacePeer) readWatchdog() {
	for !peer.wdReset.Load() {
		time.Sleep(time.Second)
		if peer.wdReset.Load() {
			break
		}
		peer.Mutex.RLock()
		lastRead := peer.lastRead
		lastWrite := peer.lastWrite
		conn := peer.conn
		peer.Mutex.RUnlock()
		if conn == nil {
			return
		}
		if time.Since(lastRead) > time.Duration(I2PProbeAfterSec*2)*time.Second {
			peer.tunnelState.Store(i2pTunnelStateStale)
		} else {
			peer.tunnelState.Store(i2pTunnelStateActive)
		}
		if time.Since(lastWrite) > time.Duration(I2PProbeAfterSec)*time.Second {
			_, _ = conn.Write([]byte{HDLCFlag, HDLCFlag})
		}
		if time.Since(lastRead) > time.Duration(i2pReadTimeoutSec)*time.Second {
			debug.Log(debug.DebugError, "I2P peer unresponsive, restarting", "name", peer.Name)
			_ = conn.Close()
			return
		}
	}
}

func (peer *I2PInterfacePeer) teardown() {
	if peer.initiator && !peer.Detached {
		debug.Log(debug.DebugError, "I2P peer unrecoverable error", "name", peer.Name)
		if peer.parent != nil && peer.parent.ctx != nil && peer.parent.ctx.PanicOnInterfaceError {
			panic("I2P interface unrecoverable error: " + peer.Name)
		}
	}
	peer.closeStream()
	peer.Mutex.Lock()
	peer.Online = false
	peer.Out = false
	peer.In = false
	peer.Mutex.Unlock()

	if peer.parent != nil {
		peer.parent.removeSpawnedPeer(peer)
	}
	if !peer.initiator && peer.parent != nil && peer.parent.ctx != nil {
		if peer.parent.ctx.VoidTunnel != nil {
			peer.parent.ctx.VoidTunnel(peer)
		}
		if peer.parent.ctx.UnregisterPeer != nil {
			peer.parent.ctx.UnregisterPeer(peer.GetName())
		}
	}
}

func (p *I2PInterface) removeSpawnedPeer(peer *I2PInterfacePeer) {
	p.spawnMu.Lock()
	defer p.spawnMu.Unlock()
	for i, sp := range p.spawned {
		if sp == peer {
			p.spawned = append(p.spawned[:i], p.spawned[i+1:]...)
			return
		}
	}
}
