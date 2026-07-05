// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/i2p"
)

const (
	i2pReconnectWait   = 15 * time.Second
	i2pReadTimeoutSec  = (I2PProbeIntervalSec*I2PProbesCount + I2PProbeAfterSec) * 2
	i2pBitrateGuess    = 256 * 1000
	i2pServerRetryWait = 15 * time.Second
	i2pTunnelRetryWait = 8 * time.Second
)

const (
	i2pTunnelStateInit   = 0x00
	i2pTunnelStateActive = 0x01
	i2pTunnelStateStale  = 0x02
)

// FromConfigContext carries runtime dependencies for interface types that
// need storage paths, transport identity, or dynamic peer registration.
type FromConfigContext struct {
	I2PStoragePath string
	TransportID    []byte
	RegisterPeer   func(name string, peer common.NetworkInterface) error
	SetupPeer      func(peer common.NetworkInterface)
}

// I2PInterface is the parent listener for inbound I2P peers and optional SAM
// server tunnel publication.
type I2PInterface struct {
	BaseInterface
	controller  *i2p.Controller
	connectable bool
	samAddress  string
	b32         string
	bindPort    int
	listener    net.Listener
	spawned     []*I2PInterfacePeer
	spawnMu     sync.Mutex
	ctx         *FromConfigContext
	transportID []byte
	serverDone  chan struct{}
	serverStop  sync.Once
	acceptDone  chan struct{}
	acceptStop  sync.Once
}

// I2PInterfacePeer is a logical Reticulum interface over one I2P stream.
type I2PInterfacePeer struct {
	BaseInterface
	parent            *I2PInterface
	conn              net.Conn
	targetDest        string
	initiator         bool
	reconnecting      bool
	neverConnected    bool
	awaitingTunnel    bool
	localPort         int
	kissFraming       bool
	maxReconnectTries int
	writing           bool
	lastRead          time.Time
	lastWrite         time.Time
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
		BaseInterface: NewBaseInterface(name, common.IFTypeI2P, cfg.Enabled),
		controller:    ctrl,
		connectable:   cfg.I2PConnectable,
		samAddress:    sam,
		bindPort:      port,
		ctx:           ctx,
		transportID:   transportID,
		serverDone:    make(chan struct{}),
		acceptDone:    make(chan struct{}),
	}
	parent.In = true
	parent.Out = false
	parent.MTU = DefaultMTU
	parent.Bitrate = i2pBitrateGuess
	parent.Mode = common.IFModeFull
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
		return err
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
	for _, peer := range p.spawned {
		_ = peer.Stop()
	}
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
		} else if dest != nil {
			p.b32 = dest.Base32()
			p.Mutex.Lock()
			p.Online = true
			p.Mutex.Unlock()
		}
		select {
		case <-p.serverDone:
			return
		case <-time.After(i2pServerRetryWait):
		}
	}
}

func (p *I2PInterface) registerSpawnedPeer(peer *I2PInterfacePeer) {
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

func NewI2PInterfacePeer(parent *I2PInterface, name, targetDest string, maxReconnect int) *I2PInterfacePeer {
	if maxReconnect == 0 {
		maxReconnect = -1
	}
	peer := &I2PInterfacePeer{
		BaseInterface:     NewBaseInterface(name, common.IFTypeI2P, parent.Enabled),
		parent:            parent,
		targetDest:        targetDest,
		initiator:         true,
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
	go peer.tunnelSetupLoop()
	return peer
}

func newI2PInterfacePeerAccepted(parent *I2PInterface, name string, conn net.Conn) *I2PInterfacePeer {
	peer := &I2PInterfacePeer{
		BaseInterface: NewBaseInterface(name, common.IFTypeI2P, parent.Enabled),
		parent:        parent,
		conn:          conn,
		initiator:     false,
		done:          make(chan struct{}),
	}
	peer.In = true
	peer.Out = true
	peer.MTU = DefaultMTU
	peer.Bitrate = i2pBitrateGuess
	peer.Mode = common.IFModeFull
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
	peer.Mutex.Lock()
	peer.Online = false
	if peer.conn != nil {
		_ = peer.conn.Close()
		peer.conn = nil
	}
	peer.Mutex.Unlock()
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

func (peer *I2PInterfacePeer) tunnelSetupLoop() {
	for peer.awaitingTunnel {
		select {
		case <-peer.done:
			return
		default:
		}
		port, err := peer.parent.controller.FreePort()
		if err != nil {
			debug.Log(debug.DebugError, "I2P peer free port failed", "name", peer.Name, "error", err)
			time.Sleep(i2pTunnelRetryWait)
			continue
		}
		peer.localPort = port
		_, err = peer.parent.controller.StartClientTunnel(peer.targetDest, port)
		if err != nil {
			debug.Log(debug.DebugError, "I2P client tunnel failed", "name", peer.Name, "error", err)
			time.Sleep(i2pTunnelRetryWait)
			continue
		}
		peer.awaitingTunnel = false
	}
	time.Sleep(2 * time.Second)
	if !peer.connect(true) {
		go peer.reconnect()
		return
	}
	go peer.readLoop()
}

func (peer *I2PInterfacePeer) connect(initial bool) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(peer.localPort)), TCPConnectTimeout)
	if err != nil {
		if initial && !peer.awaitingTunnel {
			debug.Log(debug.DebugError, "I2P peer initial connect failed", "name", peer.Name, "error", err)
		}
		return false
	}
	_ = setI2PConnTimeouts(conn)
	peer.Mutex.Lock()
	peer.conn = conn
	peer.Online = true
	peer.writing = false
	peer.neverConnected = false
	peer.Mutex.Unlock()
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
		if peer.connect(false) {
			break
		}
	}
	if !peer.neverConnected {
		debug.Log(debug.DebugInfo, "I2P peer re-established connection", "name", peer.Name)
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
	for peer.writing {
		time.Sleep(time.Millisecond)
	}
	peer.writing = true
	defer func() { peer.writing = false }()

	var frame []byte
	if peer.kissFraming {
		frame = append([]byte{KISSFend, KISSCmdData}, escapeKISS(data)...)
		frame = append(frame, KISSFend)
	} else {
		frame = append([]byte{HDLCFlag}, escapeHDLC(data)...)
		frame = append(frame, HDLCFlag)
	}
	_, err := conn.Write(frame)
	if err == nil {
		peer.Mutex.Lock()
		peer.lastWrite = time.Now()
		peer.TxBytes += uint64(len(frame))
		peer.TxPackets++
		peer.Mutex.Unlock()
		if peer.parent != nil {
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

	inFrame := false
	escape := false
	dataBuffer := make([]byte, 0, peer.MTU)
	maxFrame := 2*peer.MTU + 32

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

		for i := range n {
			b := buf[i]
			if peer.kissFraming {
				if inFrame && b == KISSFend {
					inFrame = false
					peer.deliverFrame(dataBuffer)
					dataBuffer = dataBuffer[:0]
					continue
				}
				if b == KISSFend {
					inFrame = true
					dataBuffer = dataBuffer[:0]
					continue
				}
				if inFrame && len(dataBuffer) < peer.MTU {
					if b == KISSFesc {
						escape = true
						continue
					}
					if escape {
						if b == KISSTFend {
							b = KISSFend
						} else if b == KISSTFesc {
							b = KISSFesc
						}
						escape = false
					}
					dataBuffer = append(dataBuffer, b)
				}
				continue
			}
			if b == HDLCFlag {
				if inFrame && len(dataBuffer) > 0 {
					peer.deliverFrame(dataBuffer)
					dataBuffer = dataBuffer[:0]
				}
				inFrame = !inFrame
				continue
			}
			if !inFrame {
				continue
			}
			if b == HDLCEsc {
				escape = true
				continue
			}
			if escape {
				b ^= HDLCEscMask
				escape = false
			}
			if len(dataBuffer) >= maxFrame {
				dataBuffer = dataBuffer[:0]
				inFrame = false
				continue
			}
			dataBuffer = append(dataBuffer, b)
		}
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
	if peer.parent != nil {
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
	}
	peer.Mutex.Lock()
	peer.Online = false
	peer.Out = false
	peer.In = false
	peer.Mutex.Unlock()
}

const KISSCmdData = 0x00
