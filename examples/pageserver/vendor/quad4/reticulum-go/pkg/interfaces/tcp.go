// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

type TCPClientInterface struct {
	BaseInterface
	conn              net.Conn
	targetAddr        string
	targetPort        int
	kissFraming       bool
	i2pTunneled       bool
	initiator         bool
	reconnecting      bool
	neverConnected    bool
	writing           bool
	maxReconnectTries int
	packetBuffer      []byte
	done              chan struct{}
	stopOnce          sync.Once
}

func NewTCPClientInterface(name string, targetHost string, targetPort int, kissFraming bool, i2pTunneled bool, enabled bool) (*TCPClientInterface, error) {
	tc := &TCPClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeTCP, enabled),
		targetAddr:        targetHost,
		targetPort:        targetPort,
		kissFraming:       kissFraming,
		i2pTunneled:       i2pTunneled,
		initiator:         true,
		maxReconnectTries: ReconnectWait * TCPProbesCount,
		packetBuffer:      make([]byte, 0),
		neverConnected:    true,
		done:              make(chan struct{}),
	}

	if enabled {
		// Startup should not block on remote reachability. Connection
		// establishment is handled asynchronously by reconnect().
		go tc.reconnect()
	}

	return tc, nil
}

func (tc *TCPClientInterface) Start() error {
	tc.Mutex.Lock()
	if !tc.Enabled || tc.Detached {
		tc.Mutex.Unlock()
		return fmt.Errorf("interface not enabled or detached")
	}

	if tc.conn != nil {
		tc.Online = true
		go tc.readLoop()
		tc.Mutex.Unlock()
		return nil
	}

	// Only recreate done if it's nil or was closed
	select {
	case <-tc.done:
		tc.done = make(chan struct{})
		tc.stopOnce = sync.Once{}
	default:
		if tc.done == nil {
			tc.done = make(chan struct{})
			tc.stopOnce = sync.Once{}
		}
	}
	tc.Mutex.Unlock()

	// Do not block startup waiting on remote availability.
	go tc.reconnect()
	return nil
}

func (tc *TCPClientInterface) Stop() error {
	tc.Mutex.Lock()
	tc.Enabled = false
	tc.Online = false
	if tc.conn != nil {
		_ = tc.conn.Close()
		tc.conn = nil
	}
	tc.Mutex.Unlock()

	tc.stopOnce.Do(func() {
		if tc.done != nil {
			close(tc.done)
		}
	})

	return nil
}

func (tc *TCPClientInterface) ProcessOutgoing(data []byte) error {
	tc.Mutex.RLock()
	online := tc.Online
	tc.Mutex.RUnlock()

	if !online {
		return fmt.Errorf("interface offline")
	}

	tc.writing = true
	defer func() { tc.writing = false }()

	// For TCP connections, use HDLC framing
	var frame []byte
	frame = append([]byte{HDLCFlag}, escapeHDLC(data)...)
	frame = append(frame, HDLCFlag)

	debug.Log(debug.DebugAll, "TCP interface writing to network", "name", tc.Name, "bytes", len(frame))

	tc.Mutex.RLock()
	conn := tc.conn
	tc.Mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection closed")
	}

	_, err := conn.Write(frame)
	if err != nil {
		debug.Log(debug.DebugCritical, "TCP interface write failed", "name", tc.Name, "error", err)
	}
	return err
}

func (tc *TCPClientInterface) Send(data []byte, address string) error {
	debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", tc.Name, "bytes", len(data), "address", address)

	masked, err := common.ApplyIFACOutbound(tc, data)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to mask outgoing packet for IFAC", "name", tc.Name, "error", err)
		return err
	}

	if err := tc.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugCritical, "Interface failed to send data", "name", tc.Name, "error", err)
		return err
	}

	tc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (tc *TCPClientInterface) readLoop() {
	buffer := make([]byte, tc.MTU)
	inFrame := false
	escape := false
	dataBuffer := make([]byte, 0, tc.MTU)
	maxHDLC := 2*tc.MTU + 32
	if maxHDLC < 256 {
		maxHDLC = 2048
	}

	for {
		tc.Mutex.RLock()
		conn := tc.conn
		done := tc.done
		tc.Mutex.RUnlock()

		if conn == nil {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		n, err := conn.Read(buffer)
		if err != nil {
			tc.Mutex.Lock()
			tc.Online = false
			detached := tc.Detached
			initiator := tc.initiator
			tc.Mutex.Unlock()

			if initiator && !detached {
				go tc.reconnect()
			} else {
				tc.teardown()
			}
			return
		}

		for i := range n {
			b := buffer[i]

			if b == HDLCFlag {
				if inFrame && len(dataBuffer) > 0 {
					tc.handlePacket(dataBuffer)
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

			if len(dataBuffer) >= maxHDLC {
				dataBuffer = dataBuffer[:0]
				inFrame = false
				escape = false
				continue
			}
			dataBuffer = append(dataBuffer, b)
		}
	}
}

func (tc *TCPClientInterface) handlePacket(data []byte) {
	if len(data) < 1 {
		debug.Log(debug.DebugAll, "Received invalid packet: empty")
		return
	}

	tc.Mutex.Lock()
	lastRx := time.Now()
	tc.lastRx = lastRx
	tc.Mutex.Unlock()

	debug.Log(debug.DebugAll, "Received packet", "type", fmt.Sprintf("0x%02x", data[0]), "size", len(data))

	tc.ProcessIncoming(data)
}

func (tc *TCPClientInterface) teardown() {
	tc.Online = false
	tc.In = false
	tc.Out = false
	if tc.conn != nil {
		_ = tc.conn.Close()
	}
}

// Helper functions for escaping data
func escapeHDLC(data []byte) []byte {
	need := len(data)
	for _, b := range data {
		if b == HDLCFlag || b == HDLCEsc {
			need++
		}
	}
	escaped := make([]byte, 0, need)
	for _, b := range data {
		if b == HDLCFlag || b == HDLCEsc {
			escaped = append(escaped, HDLCEsc, b^HDLCEscMask)
		} else {
			escaped = append(escaped, b)
		}
	}
	return escaped
}

func unescapeHDLC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	escape := false
	for _, b := range data {
		if escape {
			out = append(out, b^HDLCEscMask)
			escape = false
			continue
		}
		if b == HDLCEsc {
			escape = true
			continue
		}
		out = append(out, b)
	}
	return out
}

func escapeKISS(data []byte) []byte {
	escaped := make([]byte, 0, len(data)*2)
	for _, b := range data {
		if b == KISSFend {
			escaped = append(escaped, KISSFesc, KISSTFend)
		} else if b == KISSFesc {
			escaped = append(escaped, KISSFesc, KISSTFesc)
		} else {
			escaped = append(escaped, b)
		}
	}
	return escaped
}

func (tc *TCPClientInterface) SetPacketCallback(cb common.PacketCallback) {
	tc.packetCallback = cb
}

func (tc *TCPClientInterface) IsEnabled() bool {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.Enabled && tc.Online && !tc.Detached
}

func (tc *TCPClientInterface) GetName() string {
	return tc.Name
}

func (tc *TCPClientInterface) GetPacketCallback() common.PacketCallback {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.packetCallback
}

func (tc *TCPClientInterface) IsDetached() bool {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.Detached
}

func (tc *TCPClientInterface) IsOnline() bool {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.Online
}

func (tc *TCPClientInterface) reconnect() {
	tc.Mutex.Lock()
	if tc.reconnecting {
		tc.Mutex.Unlock()
		return
	}
	tc.reconnecting = true
	tc.Mutex.Unlock()

	backoff := time.Second
	maxBackoff := time.Minute * 5
	retries := 0

	for retries < tc.maxReconnectTries {
		tc.teardown()

		addr := net.JoinHostPort(tc.targetAddr, fmt.Sprintf("%d", tc.targetPort))

		conn, err := net.DialTimeout("tcp", addr, TCPConnectTimeout)
		if err == nil {
			tc.Mutex.Lock()
			tc.conn = conn
			tc.Online = true

			tc.neverConnected = false
			tc.reconnecting = false
			tc.Mutex.Unlock()

			// Set platform-specific timeouts once connected.
			switch runtime.GOOS {
			case "linux":
				if err := tc.setTimeoutsLinux(); err != nil {
					debug.Log(debug.DebugError, "Failed to set Linux TCP timeouts", "error", err)
				}
			case "darwin":
				if err := tc.setTimeoutsOSX(); err != nil {
					debug.Log(debug.DebugError, "Failed to set OSX TCP timeouts", "error", err)
				}
			}

			go tc.readLoop()
			return
		}

		debug.Log(debug.DebugVerbose, "Failed to reconnect", "target", net.JoinHostPort(tc.targetAddr, fmt.Sprintf("%d", tc.targetPort)), "attempt", retries+1, "maxTries", tc.maxReconnectTries, "error", err)

		// Wait with exponential backoff
		time.Sleep(backoff)

		// Increase backoff time exponentially
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		retries++
	}

	tc.Mutex.Lock()
	tc.reconnecting = false
	tc.Mutex.Unlock()

	tc.teardown()
	debug.Log(debug.DebugError, "Failed to reconnect after all attempts", "target", net.JoinHostPort(tc.targetAddr, fmt.Sprintf("%d", tc.targetPort)), "maxTries", tc.maxReconnectTries)
}

func (tc *TCPClientInterface) Enable() {
	tc.Mutex.Lock()
	defer tc.Mutex.Unlock()
	tc.Online = true
}

func (tc *TCPClientInterface) Disable() {
	tc.Mutex.Lock()
	defer tc.Mutex.Unlock()
	tc.Online = false
}

func (tc *TCPClientInterface) IsConnected() bool {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.conn != nil && tc.Online && !tc.reconnecting
}

func (tc *TCPClientInterface) GetRTT() time.Duration {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()

	if !tc.IsConnected() {
		return 0
	}

	if tcpConn, ok := tc.conn.(*net.TCPConn); ok {
		var rtt time.Duration
		if runtime.GOOS == "linux" {
			if info, err := tcpConn.SyscallConn(); err == nil {
				if err := info.Control(func(fd uintptr) { // #nosec G104
					rtt = platformGetRTT(fd)
				}); err != nil {
					debug.Log(debug.DebugError, "Error in SyscallConn Control", "error", err)
				}
			}
		}
		return rtt
	}

	return 0
}

type TCPServerInterface struct {
	BaseInterface
	connections map[string]net.Conn
	listener    net.Listener
	bindAddr    string
	bindPort    int
	preferIPv6  bool
	kissFraming bool
	i2pTunneled bool
	done        chan struct{}
	stopOnce    sync.Once
}

func NewTCPServerInterface(name string, bindAddr string, bindPort int, kissFraming bool, i2pTunneled bool, preferIPv6 bool) (*TCPServerInterface, error) {
	ts := &TCPServerInterface{
		BaseInterface: BaseInterface{
			Name:     name,
			Mode:     common.IFModeFull,
			Type:     common.IFTypeTCP,
			Online:   false,
			MTU:      common.DefaultMTU,
			Enabled:  true,
			Detached: false,
		},
		connections: make(map[string]net.Conn),
		bindAddr:    bindAddr,
		bindPort:    bindPort,
		preferIPv6:  preferIPv6,
		kissFraming: kissFraming,
		i2pTunneled: i2pTunneled,
		done:        make(chan struct{}),
	}

	return ts, nil
}

func (ts *TCPServerInterface) String() string {
	addr := ts.bindAddr
	if addr == "" {
		if ts.preferIPv6 {
			addr = "[::0]"
		} else {
			addr = "0.0.0.0"
		}
	}
	return fmt.Sprintf("TCPServerInterface[%s/%s:%d]", ts.Name, addr, ts.bindPort)
}

func (ts *TCPServerInterface) SetPacketCallback(callback common.PacketCallback) {
	ts.Mutex.Lock()
	defer ts.Mutex.Unlock()
	ts.packetCallback = callback
}

func (ts *TCPServerInterface) GetPacketCallback() common.PacketCallback {
	ts.Mutex.RLock()
	defer ts.Mutex.RUnlock()
	return ts.packetCallback
}

func (ts *TCPServerInterface) IsEnabled() bool {
	ts.Mutex.RLock()
	defer ts.Mutex.RUnlock()
	return ts.Enabled && ts.Online && !ts.Detached
}

func (ts *TCPServerInterface) GetName() string {
	return ts.Name
}

func (ts *TCPServerInterface) IsDetached() bool {
	ts.Mutex.RLock()
	defer ts.Mutex.RUnlock()
	return ts.Detached
}

func (ts *TCPServerInterface) IsOnline() bool {
	ts.Mutex.RLock()
	defer ts.Mutex.RUnlock()
	return ts.Online
}

func (ts *TCPServerInterface) Enable() {
	ts.Mutex.Lock()
	defer ts.Mutex.Unlock()
	ts.Online = true
}

func (ts *TCPServerInterface) Disable() {
	ts.Mutex.Lock()
	defer ts.Mutex.Unlock()
	ts.Online = false
}

func (ts *TCPServerInterface) Start() error {
	ts.Mutex.Lock()
	if ts.listener != nil {
		ts.Mutex.Unlock()
		return fmt.Errorf("TCP server already started")
	}
	// Only recreate done if it's nil or was closed
	select {
	case <-ts.done:
		ts.done = make(chan struct{})
		ts.stopOnce = sync.Once{}
	default:
		if ts.done == nil {
			ts.done = make(chan struct{})
			ts.stopOnce = sync.Once{}
		}
	}
	ts.Mutex.Unlock()

	addr := net.JoinHostPort(ts.bindAddr, fmt.Sprintf("%d", ts.bindPort))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %w", err)
	}

	ts.Mutex.Lock()
	ts.listener = listener
	ts.Online = true
	ts.Mutex.Unlock()

	// Accept connections in a goroutine
	go func() {
		for {
			ts.Mutex.RLock()
			done := ts.done
			ts.Mutex.RUnlock()

			select {
			case <-done:
				return
			default:
			}

			conn, err := listener.Accept()
			if err != nil {
				ts.Mutex.RLock()
				online := ts.Online
				ts.Mutex.RUnlock()
				if !online {
					return // Normal shutdown
				}
				debug.Log(debug.DebugError, "Error accepting connection", "error", err)
				continue
			}

			// Handle each connection in a separate goroutine
			go ts.handleConnection(conn)
		}
	}()

	return nil
}

func (ts *TCPServerInterface) Stop() error {
	ts.Mutex.Lock()
	ts.Online = false
	if ts.listener != nil {
		_ = ts.listener.Close()
		ts.listener = nil
	}
	// Close all client connections
	for addr, conn := range ts.connections {
		_ = conn.Close()
		delete(ts.connections, addr)
	}
	ts.Mutex.Unlock()

	ts.stopOnce.Do(func() {
		if ts.done != nil {
			close(ts.done)
		}
	})

	return nil
}

func (ts *TCPServerInterface) handleConnection(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	ts.Mutex.Lock()
	ts.connections[addr] = conn
	ts.Mutex.Unlock()

	defer func() {
		ts.Mutex.Lock()
		delete(ts.connections, addr)
		ts.Mutex.Unlock()
		_ = conn.Close()
	}()

	ts.readHDLCLoop(conn)
}

func (ts *TCPServerInterface) readHDLCLoop(conn net.Conn) {
	buffer := make([]byte, ts.MTU)
	inFrame := false
	escape := false
	dataBuffer := make([]byte, 0, ts.MTU)
	maxHDLC := 2*ts.MTU + 32
	if maxHDLC < 256 {
		maxHDLC = 2048
	}

	for {
		ts.Mutex.RLock()
		done := ts.done
		ts.Mutex.RUnlock()

		select {
		case <-done:
			return
		default:
		}

		n, err := conn.Read(buffer)
		if err != nil {
			return
		}

		for i := range n {
			b := buffer[i]

			if b == HDLCFlag {
				if inFrame && len(dataBuffer) > 0 {
					ts.ProcessIncoming(dataBuffer)
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

			if len(dataBuffer) >= maxHDLC {
				dataBuffer = dataBuffer[:0]
				inFrame = false
				escape = false
				continue
			}
			dataBuffer = append(dataBuffer, b)
		}
	}
}

func (ts *TCPServerInterface) ProcessOutgoing(data []byte) error {
	ts.Mutex.RLock()
	online := ts.Online
	ts.Mutex.RUnlock()

	if !online {
		return fmt.Errorf("interface offline")
	}

	var frame []byte
	if ts.kissFraming {
		frame = append([]byte{KISSFend}, escapeKISS(data)...)
		frame = append(frame, KISSFend)
	} else {
		frame = append([]byte{HDLCFlag}, escapeHDLC(data)...)
		frame = append(frame, HDLCFlag)
	}

	ts.Mutex.Lock()
	conns := make([]net.Conn, 0, len(ts.connections))
	for _, conn := range ts.connections {
		conns = append(conns, conn)
	}
	ts.Mutex.Unlock()

	for _, conn := range conns {
		if _, err := conn.Write(frame); err != nil {
			debug.Log(debug.DebugVerbose, "Error writing to connection", "address", conn.RemoteAddr(), "error", err)
		}
	}

	return nil
}

func (ts *TCPServerInterface) Send(data []byte, address string) error {
	debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", ts.Name, "bytes", len(data), "address", address)

	masked, err := common.ApplyIFACOutbound(ts, data)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to mask outgoing packet for IFAC", "name", ts.Name, "error", err)
		return err
	}

	if err := ts.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugCritical, "Interface failed to send data", "name", ts.Name, "error", err)
		return err
	}

	ts.updateBandwidthStats(uint64(len(masked)))
	return nil
}
