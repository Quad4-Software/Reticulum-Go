// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const (
	backboneHWMTU              = 1048576
	backboneServerBitrateGuess = 1_000_000_000
	backboneClientBitrateGuess = 100_000_000
)

// BackboneClientInterface is a high-throughput TCP connection with HDLC framing
// managed by the process-wide I/O hub.
type BackboneClientInterface struct {
	BaseInterface
	stream            *backbone.Stream
	conn              net.Conn
	hub               *backbone.Hub
	parent            *BackboneInterface
	targetAddr        string
	targetPort        int
	initiator         bool
	reconnecting      bool
	maxReconnectTries int
	done              chan struct{}
	stopOnce          sync.Once
}

// NewBackboneClientInterface dials cfg.TargetHost:cfg.TargetPort.
func NewBackboneClientInterface(name string, cfg *common.InterfaceConfig, hub *backbone.Hub) (*BackboneClientInterface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil interface config")
	}
	if hub == nil {
		hub = backbone.Get()
	}
	if hub == nil {
		return nil, fmt.Errorf("backbone I/O hub not initialised")
	}
	host := strings.TrimSpace(cfg.TargetHost)
	port := cfg.TargetPort
	if port == 0 {
		port = cfg.Port
	}
	if host == "" {
		return nil, fmt.Errorf("target_host required for BackboneClientInterface %q", name)
	}
	if port <= 0 {
		return nil, fmt.Errorf("target_port required for BackboneClientInterface %q", name)
	}

	maxTries := ReconnectWait * TCPProbesCount
	if cfg.MaxReconnTries > 0 {
		maxTries = cfg.MaxReconnTries
	}

	bc := &BackboneClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeBackbone, cfg.Enabled),
		hub:               hub,
		targetAddr:        host,
		targetPort:        port,
		initiator:         true,
		maxReconnectTries: maxTries,
		done:              make(chan struct{}),
	}
	bc.MTU = backboneHWMTU
	bc.Bitrate = backboneClientBitrateGuess
	bc.In = true
	bc.Out = true
	return bc, nil
}

func newSpawnedBackboneClient(parent *BackboneInterface, conn net.Conn) *BackboneClientInterface {
	name := conn.RemoteAddr().String()
	if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		name = fmt.Sprintf("Client on %s [%d]", parent.Name, tcpAddr.Port)
	}
	bc := &BackboneClientInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeBackbone, true),
		conn:          conn,
		hub:           parent.hub,
		parent:        parent,
		initiator:     false,
		done:          make(chan struct{}),
	}
	bc.MTU = parent.MTU
	bc.Bitrate = parent.Bitrate
	bc.In = parent.In
	bc.Out = parent.Out
	bc.Online = true
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}
	return bc
}

// NewBackboneFromConfig selects backbone server or client mode like reference Reticulum.
func NewBackboneFromConfig(name string, cfg *common.InterfaceConfig, hub *backbone.Hub, spawn func(*BackboneClientInterface)) (Interface, error) {
	normalizeBackboneConfig(cfg)
	if strings.TrimSpace(cfg.TargetHost) != "" {
		return NewBackboneClientInterface(name, cfg, hub)
	}
	return NewBackboneInterface(name, cfg, hub, spawn)
}

func normalizeBackboneConfig(cfg *common.InterfaceConfig) {
	if cfg == nil {
		return
	}
	if cfg.TargetPort == 0 && cfg.Port != 0 && strings.TrimSpace(cfg.TargetHost) != "" {
		cfg.TargetPort = cfg.Port
	}
}

func (bc *BackboneClientInterface) Start() error {
	bc.Mutex.Lock()
	if !bc.Enabled || bc.Detached {
		bc.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if bc.stream != nil {
		bc.Online = true
		bc.Mutex.Unlock()
		return nil
	}
	select {
	case <-bc.done:
		bc.done = make(chan struct{})
		bc.stopOnce = sync.Once{}
	default:
	}
	bc.Mutex.Unlock()

	if bc.conn != nil {
		return bc.attachStream()
	}
	if bc.initiator {
		go bc.reconnect()
	}
	return nil
}

func (bc *BackboneClientInterface) Stop() error {
	bc.Mutex.Lock()
	bc.Enabled = false
	bc.Online = false
	stream := bc.stream
	bc.stream = nil
	if bc.conn != nil {
		_ = bc.conn.Close()
		bc.conn = nil
	}
	bc.Mutex.Unlock()

	if stream != nil {
		stream.Close()
	}

	bc.stopOnce.Do(func() {
		close(bc.done)
	})
	return nil
}

func (bc *BackboneClientInterface) attachStream() error {
	conn := bc.conn
	if conn == nil {
		return fmt.Errorf("no connection")
	}
	onFrame := func(frame []byte) {
		bc.Mutex.Lock()
		bc.lastRx = time.Now()
		bc.Mutex.Unlock()
		bc.ProcessIncoming(frame)
	}
	onClose := func() {
		bc.Mutex.Lock()
		bc.Online = false
		parent := bc.parent
		initiator := bc.initiator
		detached := bc.Detached
		bc.stream = nil
		bc.Mutex.Unlock()
		if parent != nil {
			parent.removeSpawned(bc)
		}
		if initiator && !detached {
			go bc.reconnect()
		}
	}
	stream, err := bc.hub.RegisterStream(conn, bc.MTU, onFrame, onClose)
	if err != nil {
		return err
	}
	bc.Mutex.Lock()
	bc.stream = stream
	bc.Online = true
	bc.Mutex.Unlock()
	return nil
}

func (bc *BackboneClientInterface) ProcessOutgoing(data []byte) error {
	bc.Mutex.RLock()
	online := bc.Online
	stream := bc.stream
	bc.Mutex.RUnlock()
	if !online || stream == nil {
		return fmt.Errorf("interface offline")
	}
	stream.QueueSend(data)
	return nil
}

func (bc *BackboneClientInterface) Send(data []byte, address string) error {
	masked, err := common.ApplyIFACOutbound(bc, data)
	if err != nil {
		return err
	}
	if err := bc.ProcessOutgoing(masked); err != nil {
		return err
	}
	bc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (bc *BackboneClientInterface) reconnect() {
	bc.Mutex.Lock()
	if bc.reconnecting {
		bc.Mutex.Unlock()
		return
	}
	bc.reconnecting = true
	bc.Mutex.Unlock()
	defer func() {
		bc.Mutex.Lock()
		bc.reconnecting = false
		bc.Mutex.Unlock()
	}()

	backoff := time.Second
	maxBackoff := 5 * time.Minute
	retries := 0

	for retries < bc.maxReconnectTries {
		select {
		case <-bc.done:
			return
		default:
		}

		addr := net.JoinHostPort(bc.targetAddr, fmt.Sprintf("%d", bc.targetPort))
		conn, err := net.DialTimeout("tcp", addr, TCPConnectTimeout)
		if err == nil {
			bc.Mutex.Lock()
			bc.conn = conn
			bc.Mutex.Unlock()
			switch runtime.GOOS {
			case "linux":
				_ = bc.setTimeoutsLinux(conn)
			case "darwin":
				_ = bc.setTimeoutsOSX(conn)
			}
			if err := bc.attachStream(); err != nil {
				debug.Log(debug.DebugError, "backbone reconnect attach failed", "error", err)
				_ = conn.Close()
			} else {
				return
			}
		}

		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		retries++
	}
	debug.Log(debug.DebugError, "backbone reconnect exhausted", "target", bc.targetAddr)
}

func (bc *BackboneClientInterface) setTimeoutsLinux(conn net.Conn) error {
	tc, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	tmp := &TCPClientInterface{conn: tc}
	return tmp.setTimeoutsLinux()
}

func (bc *BackboneClientInterface) setTimeoutsOSX(conn net.Conn) error {
	tc, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	tmp := &TCPClientInterface{conn: tc}
	return tmp.setTimeoutsOSX()
}

func (bc *BackboneClientInterface) GetConn() net.Conn {
	bc.Mutex.RLock()
	defer bc.Mutex.RUnlock()
	return bc.conn
}

func (bc *BackboneClientInterface) IsConnected() bool {
	bc.Mutex.RLock()
	defer bc.Mutex.RUnlock()
	return bc.conn != nil && bc.Online
}

func (bc *BackboneClientInterface) GetRTT() time.Duration {
	bc.Mutex.RLock()
	conn := bc.conn
	bc.Mutex.RUnlock()
	if conn == nil {
		return 0
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok && runtime.GOOS == "linux" {
		var rtt time.Duration
		if info, err := tcpConn.SyscallConn(); err == nil {
			_ = info.Control(func(fd uintptr) {
				rtt = platformGetRTT(fd)
			})
		}
		return rtt
	}
	return 0
}
