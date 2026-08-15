// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build linux && !js

package interfaces

import (
	"fmt"
	"io"
	"math"
	"net"
	"sync"

	"github.com/mdlayher/vsock"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

const vsockBitrateGuess int64 = 1_000_000_000

// ParseVSOCKContextID converts a config integer to a vsock context ID.
// Negative values and values above math.MaxUint32 are rejected.
func ParseVSOCKContextID(v int) (uint32, error) {
	if v < 0 {
		return 0, fmt.Errorf("vsock context ID must be non-negative, got %d", v)
	}
	if uint64(v) > math.MaxUint32 {
		return 0, fmt.Errorf("vsock context ID overflows uint32, got %d", v)
	}
	return uint32(v), nil
}

// VSOCKClientInterface dials an AF_VSOCK peer and carries HDLC frames.
type VSOCKClientInterface struct {
	BaseInterface
	conn              net.Conn
	contextID         uint32
	port              uint32
	maxReconnectTries int
	done              chan struct{}
	stopOnce          sync.Once
	reconnect         *reconnectDriver
	txFrame           []byte
	txMu              sync.Mutex
	readBuf           []byte
}

// NewVSOCKClientInterface constructs a VSOCK client with unlimited reconnect by default.
func NewVSOCKClientInterface(name string, contextID uint32, port uint32, enabled bool) (*VSOCKClientInterface, error) {
	return NewVSOCKClientInterfaceWithRetries(name, contextID, port, enabled, 0)
}

// NewVSOCKClientInterfaceWithRetries constructs a VSOCK client with a reconnect limit.
func NewVSOCKClientInterfaceWithRetries(name string, contextID uint32, port uint32, enabled bool, maxTries int) (*VSOCKClientInterface, error) {
	vc := &VSOCKClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeVSOCK, enabled),
		contextID:         contextID,
		port:              port,
		maxReconnectTries: NormalizeMaxReconnectTries(maxTries),
		done:              make(chan struct{}),
		txFrame:           make([]byte, 0, DefaultMTU*2+4),
	}
	vc.MTU = DefaultMTU
	vc.Bitrate = vsockBitrateGuess
	vc.In = true
	vc.Out = true
	vc.initReconnectDriver()
	if enabled {
		vc.startReconnect()
	}
	return vc, nil
}

func (vc *VSOCKClientInterface) initReconnectDriver() {
	label := fmt.Sprintf("vsock:%d:%d", vc.contextID, vc.port)
	vc.reconnect = newReconnectDriver(label, vc.maxReconnectTries, vc.done, vc.dialSession, vc.onConnected)
	vc.reconnect.setOnExhausted(func() {
		_ = vc.Stop()
	})
}

func (vc *VSOCKClientInterface) dialSession() (net.Conn, error) {
	return vsock.Dial(vc.contextID, vc.port, nil)
}

func (vc *VSOCKClientInterface) onConnected(conn net.Conn) {
	if !vc.adoptConn(conn) {
		_ = conn.Close()
		return
	}
	go vc.readLoop()
}

func (vc *VSOCKClientInterface) adoptConn(conn net.Conn) bool {
	vc.Mutex.Lock()
	defer vc.Mutex.Unlock()
	if vc.Detached {
		return false
	}
	select {
	case <-vc.done:
		return false
	default:
	}
	vc.conn = conn
	vc.Online = true
	return true
}

// SetConnectivityHooks registers reconnect up/down callbacks.
func (vc *VSOCKClientInterface) SetConnectivityHooks(onDown, onUp func()) {
	vc.Mutex.Lock()
	reconnect := vc.reconnect
	vc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.setHooks(onDown, onUp)
	}
}

func (vc *VSOCKClientInterface) startReconnect() {
	vc.Mutex.Lock()
	reconnect := vc.reconnect
	vc.Mutex.Unlock()
	if reconnect != nil {
		reconnect.start()
	}
}

// Start begins or resumes the reconnect dial loop.
func (vc *VSOCKClientInterface) Start() error {
	vc.Mutex.Lock()
	if !vc.Enabled || vc.Detached {
		vc.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}
	if vc.conn != nil {
		vc.Online = true
		go vc.readLoop()
		vc.Mutex.Unlock()
		return nil
	}
	select {
	case <-vc.done:
		vc.done = make(chan struct{})
		vc.stopOnce = sync.Once{}
		vc.initReconnectDriver()
	default:
		if vc.done == nil {
			vc.done = make(chan struct{})
			vc.stopOnce = sync.Once{}
		}
		if vc.reconnect == nil {
			vc.initReconnectDriver()
		}
	}
	vc.Mutex.Unlock()
	vc.startReconnect()
	return nil
}

// Stop closes the session and stops reconnect.
func (vc *VSOCKClientInterface) Stop() error {
	vc.Mutex.Lock()
	vc.Enabled = false
	vc.Online = false
	if vc.conn != nil {
		_ = vc.conn.Close()
		vc.conn = nil
	}
	vc.Mutex.Unlock()
	vc.stopOnce.Do(func() {
		if vc.done != nil {
			close(vc.done)
		}
	})
	return nil
}

// ProcessOutgoing writes an HDLC-framed payload on the VSOCK connection.
func (vc *VSOCKClientInterface) ProcessOutgoing(data []byte) error {
	vc.Mutex.RLock()
	online := vc.Online
	conn := vc.conn
	vc.Mutex.RUnlock()
	if !online || conn == nil {
		return fmt.Errorf("interface offline")
	}
	vc.txMu.Lock()
	frame := appendFrameHDLC(vc.txFrame[:0], data)
	vc.txFrame = frame
	out := append([]byte(nil), frame...)
	vc.txMu.Unlock()
	_, err := conn.Write(out)
	if err != nil {
		vc.Mutex.Lock()
		vc.Online = false
		detached := vc.Detached
		reconnect := vc.reconnect
		vc.Mutex.Unlock()
		if !detached && reconnect != nil {
			vc.teardownConn()
			reconnect.notifyFailure()
		}
	}
	return err
}

// Send applies IFAC then ProcessOutgoing.
func (vc *VSOCKClientInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(vc); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(vc, data)
	if err != nil {
		return err
	}
	if err := vc.ProcessOutgoing(masked); err != nil {
		return err
	}
	vc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (vc *VSOCKClientInterface) readLoop() {
	decoder := newHDLCToggleStreamDecoder(vc.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		vc.ProcessIncoming(payload)
	})
	if cap(vc.readBuf) < vc.MTU {
		vc.readBuf = make([]byte, vc.MTU)
	}
	buffer := vc.readBuf[:vc.MTU]
	for {
		vc.Mutex.RLock()
		conn := vc.conn
		done := vc.done
		vc.Mutex.RUnlock()
		if conn == nil {
			return
		}
		select {
		case <-done:
			return
		default:
		}
		n, err := conn.Read(buffer)
		if n > 0 {
			decoder.feed(buffer[:n])
		}
		if err != nil {
			if err != io.EOF {
				debug.Log(debug.DebugVerbose, "VSOCK client read ended", "name", vc.Name, "error", err)
			}
			vc.Mutex.Lock()
			vc.Online = false
			detached := vc.Detached
			reconnect := vc.reconnect
			vc.Mutex.Unlock()
			if !detached && reconnect != nil {
				vc.teardownConn()
				reconnect.notifyFailure()
			} else {
				vc.teardownConn()
			}
			return
		}
	}
}

func (vc *VSOCKClientInterface) teardownConn() {
	vc.Mutex.Lock()
	if vc.conn != nil {
		_ = vc.conn.Close()
		vc.conn = nil
	}
	vc.Mutex.Unlock()
}

// ContextID returns the peer context ID this client dials.
func (vc *VSOCKClientInterface) ContextID() uint32 {
	return vc.contextID
}

// Port returns the peer port this client dials.
func (vc *VSOCKClientInterface) Port() uint32 {
	return vc.port
}

// VSOCKServerInterface listens for AF_VSOCK peers and fans out HDLC frames.
type VSOCKServerInterface struct {
	BaseInterface
	connections map[string]net.Conn
	listener    net.Listener
	listenCID   uint32
	port        uint32
	done        chan struct{}
	stopOnce    sync.Once
	txFrame     []byte
	txMu        sync.Mutex
	acceptWg    sync.WaitGroup
}

// NewVSOCKServerInterface constructs a VSOCK server that listens on Local CID.
// Local (1) enables same-host loopback without a guest VM.
func NewVSOCKServerInterface(name string, port uint32) (*VSOCKServerInterface, error) {
	vs := &VSOCKServerInterface{
		BaseInterface: NewBaseInterface(name, common.IFTypeVSOCK, true),
		connections:   make(map[string]net.Conn),
		listenCID:     vsock.Local,
		port:          port,
		done:          make(chan struct{}),
		txFrame:       make([]byte, 0, DefaultMTU*2+4),
	}
	vs.MTU = DefaultMTU
	vs.Bitrate = vsockBitrateGuess
	vs.In = true
	vs.Out = true
	return vs, nil
}

// SetListenContextID sets the CID used by Start. Call before Start.
// Use vsock.Local for same-host tests, or the machine CID for guest/host binds.
func (vs *VSOCKServerInterface) SetListenContextID(cid uint32) {
	vs.Mutex.Lock()
	vs.listenCID = cid
	vs.Mutex.Unlock()
}

// Start listens and accepts VSOCK connections.
func (vs *VSOCKServerInterface) Start() error {
	vs.acceptWg.Wait()
	vs.Mutex.Lock()
	if vs.listener != nil {
		vs.Mutex.Unlock()
		return fmt.Errorf("VSOCK server already started")
	}
	select {
	case <-vs.done:
		vs.done = make(chan struct{})
		vs.stopOnce = sync.Once{}
	default:
		if vs.done == nil {
			vs.done = make(chan struct{})
			vs.stopOnce = sync.Once{}
		}
	}
	listenCID := vs.listenCID
	port := vs.port
	vs.Mutex.Unlock()

	ln, err := vsock.ListenContextID(listenCID, port, nil)
	if err != nil {
		return fmt.Errorf("failed to start VSOCK server: %w", err)
	}

	vs.Mutex.Lock()
	vs.listener = ln
	if addr, ok := ln.Addr().(*vsock.Addr); ok {
		vs.port = addr.Port
		vs.listenCID = addr.ContextID
	}
	vs.Online = true
	vs.Mutex.Unlock()

	vs.acceptWg.Go(func() {
		vs.acceptLoop(ln)
	})
	return nil
}

func (vs *VSOCKServerInterface) acceptLoop(ln net.Listener) {
	for {
		vs.Mutex.RLock()
		done := vs.done
		vs.Mutex.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		conn, err := ln.Accept()
		if err != nil {
			vs.Mutex.RLock()
			online := vs.Online
			vs.Mutex.RUnlock()
			if !online {
				return
			}
			select {
			case <-done:
				return
			default:
			}
			debug.Log(debug.DebugVerbose, "VSOCK accept error", "name", vs.Name, "error", err)
			continue
		}
		go vs.handleConn(conn)
	}
}

func (vs *VSOCKServerInterface) handleConn(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	vs.Mutex.Lock()
	vs.connections[addr] = conn
	vs.Mutex.Unlock()
	defer func() {
		vs.Mutex.Lock()
		delete(vs.connections, addr)
		vs.Mutex.Unlock()
		_ = conn.Close()
	}()
	vs.readHDLCLoop(conn)
}

// SessionCount returns the number of active VSOCK sessions.
func (vs *VSOCKServerInterface) SessionCount() int {
	vs.Mutex.RLock()
	defer vs.Mutex.RUnlock()
	return len(vs.connections)
}

func (vs *VSOCKServerInterface) readHDLCLoop(conn net.Conn) {
	decoder := newHDLCToggleStreamDecoder(vs.MTU, func(payload []byte) {
		if len(payload) == 0 {
			return
		}
		vs.ProcessIncoming(payload)
	})
	buf := make([]byte, vs.MTU)
	for {
		vs.Mutex.RLock()
		done := vs.done
		vs.Mutex.RUnlock()
		select {
		case <-done:
			return
		default:
		}
		n, err := conn.Read(buf)
		if n > 0 {
			decoder.feed(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Stop closes the listener and all sessions.
func (vs *VSOCKServerInterface) Stop() error {
	vs.Mutex.Lock()
	vs.Online = false
	ln := vs.listener
	vs.listener = nil
	conns := make([]net.Conn, 0, len(vs.connections))
	for addr, conn := range vs.connections {
		conns = append(conns, conn)
		delete(vs.connections, addr)
	}
	vs.Mutex.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	for _, conn := range conns {
		_ = conn.Close()
	}
	vs.stopOnce.Do(func() {
		if vs.done != nil {
			close(vs.done)
		}
	})
	vs.acceptWg.Wait()
	return nil
}

// ProcessOutgoing fans out an HDLC frame to all sessions.
func (vs *VSOCKServerInterface) ProcessOutgoing(data []byte) error {
	vs.Mutex.RLock()
	online := vs.Online
	vs.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}
	vs.txMu.Lock()
	frame := appendFrameHDLC(vs.txFrame[:0], data)
	vs.txFrame = frame
	out := append([]byte(nil), frame...)
	vs.txMu.Unlock()
	vs.Mutex.Lock()
	conns := make([]net.Conn, 0, len(vs.connections))
	for _, c := range vs.connections {
		conns = append(conns, c)
	}
	vs.Mutex.Unlock()
	if len(conns) == 0 {
		return fmt.Errorf("no VSOCK sessions")
	}
	var lastErr error
	for _, c := range conns {
		if _, err := c.Write(out); err != nil {
			debug.Log(debug.DebugVerbose, "VSOCK server write failed", "address", c.RemoteAddr(), "error", err)
			lastErr = err
		}
	}
	return lastErr
}

// Send applies IFAC then ProcessOutgoing.
func (vs *VSOCKServerInterface) Send(data []byte, address string) error {
	_ = address
	if err := common.RejectReceiveOnly(vs); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(vs, data)
	if err != nil {
		return err
	}
	if err := vs.ProcessOutgoing(masked); err != nil {
		return err
	}
	vs.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// ListenAddr returns the bound address after Start.
func (vs *VSOCKServerInterface) ListenAddr() net.Addr {
	vs.Mutex.RLock()
	defer vs.Mutex.RUnlock()
	if vs.listener == nil {
		return nil
	}
	return vs.listener.Addr()
}

// Port returns the bound port after Start (or the configured port before).
func (vs *VSOCKServerInterface) Port() uint32 {
	vs.Mutex.RLock()
	defer vs.Mutex.RUnlock()
	return vs.port
}
