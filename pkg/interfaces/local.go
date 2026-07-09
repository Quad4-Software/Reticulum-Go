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

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const localReconnectWait = 8 * time.Second

// LocalSpawnHook is invoked when the shared-instance server accepts a client.
type LocalSpawnHook func(client *LocalClientInterface)

// LocalServerInterface listens for local shared-instance clients over TCP or
// Unix domain sockets with HDLC framing.
type LocalServerInterface struct {
	BaseInterface
	listener   net.Listener
	bindPort   int
	socketPath string
	useUnix    bool
	hub        *backbone.Hub
	clients    atomic.Int32
	spawnHook  LocalSpawnHook
	done       chan struct{}
	stopOnce   sync.Once
}

// NewLocalServerInterface binds a shared-instance listener on 127.0.0.1:port
// or on an abstract Unix socket @rns/<name> when useUnix is true.
func NewLocalServerInterface(port int, socketPath string, useUnix bool, spawn LocalSpawnHook, hub *backbone.Hub) (*LocalServerInterface, error) {
	if spawn == nil {
		return nil, fmt.Errorf("local server requires spawn hook")
	}
	if hub == nil {
		hub = backbone.Get()
	}
	ls := &LocalServerInterface{
		BaseInterface: NewBaseInterface("Reticulum", common.IFTypeUnix, true),
		bindPort:      port,
		socketPath:    socketPath,
		useUnix:       useUnix,
		hub:           hub,
		spawnHook:     spawn,
		done:          make(chan struct{}),
	}
	ls.Name = "Reticulum"
	ls.In = true
	ls.Out = true
	ls.Bitrate = 1_000_000_000
	ls.MTU = 262144
	ls.Online = false
	return ls, nil
}

func (ls *LocalServerInterface) String() string {
	if ls.useUnix {
		return fmt.Sprintf("Shared Instance[@rns/%s]", ls.socketPath)
	}
	return fmt.Sprintf("Shared Instance[%d]", ls.bindPort)
}

func (ls *LocalServerInterface) Start() error {
	ls.Mutex.Lock()
	if ls.listener != nil {
		ls.Mutex.Unlock()
		return nil
	}
	select {
	case <-ls.done:
		ls.done = make(chan struct{})
		ls.stopOnce = sync.Once{}
	default:
	}
	ls.Mutex.Unlock()

	var (
		ln  net.Listener
		err error
	)
	if ls.useUnix {
		name := ls.socketPath
		if name == "" {
			name = "default"
		}
		ln, err = net.Listen("unix", "@"+"rns/"+name)
	} else {
		ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(ls.bindPort)))
	}
	if err != nil {
		return err
	}

	ls.Mutex.Lock()
	ls.listener = ln
	ls.Online = true
	ls.Mutex.Unlock()

	if ls.hub != nil {
		return ls.hub.RegisterListener(ln, ls.handleConnection)
	}
	go ls.acceptLoop()
	return nil
}

func (ls *LocalServerInterface) Stop() error {
	ls.Mutex.Lock()
	ls.Online = false
	if ls.listener != nil {
		_ = ls.listener.Close()
		ls.listener = nil
	}
	ls.Mutex.Unlock()
	ls.stopOnce.Do(func() {
		close(ls.done)
	})
	return nil
}

func (ls *LocalServerInterface) acceptLoop() {
	for {
		ls.Mutex.RLock()
		ln := ls.listener
		done := ls.done
		ls.Mutex.RUnlock()
		if ln == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		conn, err := ln.Accept()
		if err != nil {
			ls.Mutex.RLock()
			online := ls.Online
			ls.Mutex.RUnlock()
			if !online {
				return
			}
			debug.Log(debug.DebugError, "Local shared instance accept error", "error", err)
			continue
		}
		ls.handleConnection(conn)
	}
}

func (ls *LocalServerInterface) handleConnection(conn net.Conn) {
	idx := int(ls.clients.Add(1))
	name := conn.RemoteAddr().String()
	if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		name = strconv.Itoa(tcpAddr.Port)
	}
	_ = idx
	client := newLocalClientFromConn(name, conn, ls, true)
	client.Out = ls.Out
	client.In = ls.In
	client.Bitrate = ls.Bitrate
	client.MTU = ls.MTU
	ls.spawnHook(client)
	if ls.hub != nil {
		_ = client.attachToHub(ls.hub)
		return
	}
	go client.readLoop()
}

func (ls *LocalServerInterface) ProcessOutgoing([]byte) error {
	return nil
}

func (ls *LocalServerInterface) Send(data []byte, address string) error {
	return ls.ProcessOutgoing(data)
}

func (ls *LocalServerInterface) GetConn() net.Conn { return nil }

func (ls *LocalServerInterface) SendPathRequest([]byte) error { return nil }

func (ls *LocalServerInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (ls *LocalServerInterface) GetBandwidthAvailable() bool { return true }

// LocalClientInterface connects to a local shared Reticulum instance.
type LocalClientInterface struct {
	BaseInterface
	conn            net.Conn
	stream          *backbone.Stream
	parent          *LocalServerInterface
	sharedInitiator bool
	reconnecting    bool
	done            chan struct{}
	stopOnce        sync.Once
	targetPort      int
	socketPath      string
	useUnix         bool
	hub             *backbone.Hub
	onDisconnect    func()
	onReconnect     func()
	txFrame         []byte
	readBuf         []byte
}

// NewLocalClientInterface dials the shared instance on localhost.
func NewLocalClientInterface(port int, socketPath string, useUnix bool, hub *backbone.Hub) (*LocalClientInterface, error) {
	lc := &LocalClientInterface{
		BaseInterface:   NewBaseInterface("Local shared instance", common.IFTypeUnix, true),
		targetPort:      port,
		socketPath:      socketPath,
		useUnix:         useUnix,
		hub:             hub,
		sharedInitiator: true,
		done:            make(chan struct{}),
		txFrame:         make([]byte, 0, 512),
		readBuf:         nil,
	}
	lc.In = true
	lc.Out = true
	lc.Bitrate = 1_000_000_000
	lc.MTU = 262144
	lc.readBuf = make([]byte, lc.MTU)
	return lc, nil
}

func newLocalClientFromConn(name string, conn net.Conn, parent *LocalServerInterface, spawned bool) *LocalClientInterface {
	lc := &LocalClientInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeUnix, true),
		conn:          conn,
		parent:        parent,
		done:          make(chan struct{}),
		txFrame:       make([]byte, 0, 512),
	}
	lc.In = true
	lc.Out = true
	lc.Bitrate = 1_000_000_000
	lc.MTU = 262144
	lc.readBuf = make([]byte, lc.MTU)
	lc.Online = true
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	if spawned {
		lc.sharedInitiator = false
	}
	return lc
}

func (lc *LocalClientInterface) IsSharedInstanceClient() bool {
	return lc.sharedInitiator
}

func (lc *LocalClientInterface) ShouldIngressLimitPR() bool { return false }

func (lc *LocalClientInterface) String() string {
	if lc.useUnix || lc.parent != nil {
		path := lc.socketPath
		if path == "" {
			path = "default"
		}
		return fmt.Sprintf("LocalInterface[@rns/%s]", path)
	}
	return fmt.Sprintf("LocalInterface[%d]", lc.targetPort)
}

func (lc *LocalClientInterface) Start() error {
	if lc.conn != nil {
		if lc.hub != nil {
			return lc.attachToHub(lc.hub)
		}
		go lc.readLoop()
		return nil
	}
	if err := lc.connect(); err != nil {
		return err
	}
	if lc.hub != nil {
		return lc.attachToHub(lc.hub)
	}
	go lc.readLoop()
	return nil
}

func (lc *LocalClientInterface) attachToHub(hub *backbone.Hub) error {
	conn := lc.conn
	if conn == nil {
		return fmt.Errorf("local client has no connection")
	}
	onFrame := func(frame []byte) {
		lc.ProcessIncoming(frame)
	}
	onClose := func() {
		lc.handleDisconnect()
	}
	stream, err := hub.RegisterStream(conn, lc.MTU, onFrame, onClose)
	if err != nil {
		return err
	}
	lc.stream = stream
	return nil
}

func (lc *LocalClientInterface) connect() error {
	var (
		conn net.Conn
		err  error
	)
	if lc.useUnix {
		name := lc.socketPath
		if name == "" {
			name = "default"
		}
		conn, err = net.Dial("unix", "@"+"rns/"+name)
	} else {
		conn, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(lc.targetPort)))
	}
	if err != nil {
		return err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	lc.Mutex.Lock()
	lc.conn = conn
	lc.Online = true
	lc.Mutex.Unlock()
	return nil
}

func (lc *LocalClientInterface) Stop() error {
	lc.Mutex.Lock()
	lc.Enabled = false
	lc.Online = false
	stream := lc.stream
	lc.stream = nil
	if lc.conn != nil {
		_ = lc.conn.Close()
		lc.conn = nil
	}
	lc.Mutex.Unlock()
	if stream != nil {
		stream.Close()
	}
	lc.stopOnce.Do(func() {
		close(lc.done)
	})
	return nil
}

func (lc *LocalClientInterface) ProcessOutgoing(data []byte) error {
	lc.Mutex.RLock()
	conn := lc.conn
	stream := lc.stream
	online := lc.Online
	lc.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("local interface offline")
	}
	if stream != nil {
		stream.QueueSend(data)
		return nil
	}
	frame := appendFrameHDLC(lc.txFrame[:0], data)
	lc.txFrame = frame
	_, err := conn.Write(frame)
	return err
}

func (lc *LocalClientInterface) Send(data []byte, address string) error {
	masked, err := common.ApplyIFACOutbound(lc, data)
	if err != nil {
		return err
	}
	if err := lc.ProcessOutgoing(masked); err != nil {
		return err
	}
	lc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (lc *LocalClientInterface) readLoop() {
	lc.runHDLCLoop(func(frame []byte) {
		lc.ProcessIncoming(frame)
	})
}

func (lc *LocalClientInterface) runHDLCLoop(onFrame func([]byte)) {
	decoder := newHDLCStreamDecoder(lc.MTU, onFrame)
	if cap(lc.readBuf) < lc.MTU {
		lc.readBuf = make([]byte, lc.MTU)
	}
	buffer := lc.readBuf[:lc.MTU]

	for {
		lc.Mutex.RLock()
		conn := lc.conn
		done := lc.done
		lc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		n, err := conn.Read(buffer)
		if n == 0 {
			if err != nil {
				lc.handleDisconnect()
				return
			}
			continue
		}
		decoder.feed(buffer[:n])
		if err != nil {
			lc.handleDisconnect()
			return
		}
	}
}

func (lc *LocalClientInterface) handleDisconnect() {
	lc.Mutex.Lock()
	lc.Online = false
	if lc.conn != nil {
		_ = lc.conn.Close()
		lc.conn = nil
	}
	lc.Mutex.Unlock()
	if lc.sharedInitiator && lc.Enabled && !lc.Detached {
		if lc.onDisconnect != nil {
			lc.onDisconnect()
		}
		go lc.reconnect()
		return
	}
}

func (lc *LocalClientInterface) reconnect() {
	if lc.reconnecting {
		return
	}
	lc.reconnecting = true
	defer func() { lc.reconnecting = false }()
	for lc.Enabled && !lc.Detached {
		time.Sleep(localReconnectWait)
		if err := lc.connect(); err != nil {
			debug.Log(debug.DebugTrace, "Local shared instance reconnect failed", "error", err)
			continue
		}
		debug.Log(debug.DebugInfo, "Reconnected to local shared instance")
		if lc.onReconnect != nil {
			lc.onReconnect()
		}
		if lc.hub != nil {
			if err := lc.attachToHub(lc.hub); err != nil {
				debug.Log(debug.DebugError, "local reconnect hub attach failed", "error", err)
				continue
			}
			return
		}
		lc.readLoop()
		return
	}
}

func (lc *LocalClientInterface) GetConn() net.Conn {
	lc.Mutex.RLock()
	defer lc.Mutex.RUnlock()
	return lc.conn
}

func (lc *LocalClientInterface) SendPathRequest([]byte) error { return nil }

func (lc *LocalClientInterface) SendLinkPacket([]byte, []byte, time.Time) error { return nil }

func (lc *LocalClientInterface) GetBandwidthAvailable() bool {
	lc.Mutex.RLock()
	defer lc.Mutex.RUnlock()
	return lc.Online && lc.conn != nil
}

func (lc *LocalClientInterface) SetDisconnectHooks(onDisconnect, onReconnect func()) {
	lc.onDisconnect = onDisconnect
	lc.onReconnect = onReconnect
}

func (lc *LocalClientInterface) ParentClients() *atomic.Int32 {
	if lc.parent == nil {
		return nil
	}
	return &lc.parent.clients
}
