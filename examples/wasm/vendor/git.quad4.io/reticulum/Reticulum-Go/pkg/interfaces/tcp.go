// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/protect"
)

type TCPClientInterface struct {
	BaseInterface
	conn              net.Conn
	targetAddr        string
	targetPort        int
	kissFraming       bool
	i2pTunneled       bool
	initiator         bool
	neverConnected    bool
	sendMu            sync.Mutex
	maxReconnectTries int
	packetBuffer      []byte
	done              chan struct{}
	stopOnce          sync.Once
	reconnect         *reconnectDriver
	wantsTunnel       bool
	tunnelID          []byte
	synthesizeTunnel  func(TunnelPeer)
	txFrame           []byte
	readBuf           []byte

	AutoconnectHash   []byte
	AutoconnectSource []byte
}

func NewTCPClientInterface(name string, targetHost string, targetPort int, kissFraming bool, i2pTunneled bool, enabled bool) (*TCPClientInterface, error) {
	return NewTCPClientInterfaceWithRetries(name, targetHost, targetPort, kissFraming, i2pTunneled, enabled, 0)
}

func NewTCPClientInterfaceWithRetries(name string, targetHost string, targetPort int, kissFraming bool, i2pTunneled bool, enabled bool, maxReconnectTries int) (*TCPClientInterface, error) {
	tc := &TCPClientInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeTCP, enabled),
		targetAddr:        targetHost,
		targetPort:        targetPort,
		kissFraming:       kissFraming,
		i2pTunneled:       i2pTunneled,
		initiator:         true,
		maxReconnectTries: NormalizeMaxReconnectTries(maxReconnectTries),
		packetBuffer:      make([]byte, 0),
		neverConnected:    true,
		done:              make(chan struct{}),
		wantsTunnel:       !kissFraming,
		txFrame:           make([]byte, 0, DefaultMTU*2+4),
	}
	tc.initReconnectDriver()
	tc.Bitrate = BitrateGuess
	tc.SetTunnelsAllowed(true)
	tc.SetLosslessMedium(true)

	if enabled {
		tc.startReconnect()
	}

	return tc, nil
}

func (tc *TCPClientInterface) SetTunnelSynth(fn func(TunnelPeer)) {
	tc.Mutex.Lock()
	tc.synthesizeTunnel = fn
	tc.Mutex.Unlock()
}

func (tc *TCPClientInterface) InterfaceHash() []byte {
	return InterfaceHashFromName(tc.Name)
}

func (tc *TCPClientInterface) WantsTunnel() bool {
	return tc.wantsTunnel
}

func (tc *TCPClientInterface) SetWantsTunnel(v bool) {
	tc.wantsTunnel = v
}

func (tc *TCPClientInterface) TunnelID() []byte {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return append([]byte(nil), tc.tunnelID...)
}

func (tc *TCPClientInterface) SetTunnelID(id []byte) {
	tc.Mutex.Lock()
	tc.tunnelID = append(tc.tunnelID[:0], id...)
	tc.Mutex.Unlock()
}

func (tc *TCPClientInterface) onConnected(conn net.Conn) {
	if !tc.adoptConn(conn) {
		_ = conn.Close()
		return
	}
	applyClientTCPTimeouts(tc)
	tc.Mutex.RLock()
	synth := tc.synthesizeTunnel
	tc.Mutex.RUnlock()
	// RNS 1.6.1: each new connection restarts tunnel synthesis state.
	if !tc.kissFraming {
		tc.SetWantsTunnel(true)
	}
	tc.SetRemoteTransportHash(nil)
	tc.SetSynthesisReceived(false)
	if tc.WantsTunnel() && synth != nil {
		synth(tc)
	}
	go tc.readLoop()
}

func (tc *TCPClientInterface) initReconnectDriver() {
	label := net.JoinHostPort(tc.targetAddr, fmt.Sprintf("%d", tc.targetPort))
	tc.reconnect = newReconnectDriver(label, tc.maxReconnectTries, tc.done, tcpDialTarget(tc.targetAddr, tc.targetPort), tc.onConnected)
	tc.reconnect.setOnExhausted(func() {
		_ = tc.Stop()
	})
}

func (tc *TCPClientInterface) adoptConn(conn net.Conn) bool {
	tc.Mutex.Lock()
	defer tc.Mutex.Unlock()
	if tc.Detached {
		return false
	}
	select {
	case <-tc.done:
		return false
	default:
	}
	tc.conn = conn
	tc.Online = true
	tc.neverConnected = false
	return true
}

func (tc *TCPClientInterface) SetConnectivityHooks(onDown, onUp func()) {
	if tc.reconnect != nil {
		tc.reconnect.setHooks(onDown, onUp)
	}
}

func (tc *TCPClientInterface) startReconnect() {
	if tc.reconnect != nil {
		tc.reconnect.start()
	}
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

	// Construction with Enabled already owns a reconnect driver. Replacing
	// it races two dial loops on tc.conn. Only rebuild after Stop closed done.
	select {
	case <-tc.done:
		tc.done = make(chan struct{})
		tc.stopOnce = sync.Once{}
		tc.initReconnectDriver()
	default:
		if tc.done == nil {
			tc.done = make(chan struct{})
			tc.stopOnce = sync.Once{}
		}
		if tc.reconnect == nil {
			tc.initReconnectDriver()
		}
	}
	tc.Mutex.Unlock()

	tc.startReconnect()
	return nil
}

// Detach marks the client detached and closes the connection, ending any
// reconnect loop permanently (RNS 1.5.5 TCPClientInterface.detach).
func (tc *TCPClientInterface) Detach() {
	tc.Mutex.Lock()
	tc.Detached = true
	tc.Online = false
	tc.Mutex.Unlock()
	_ = tc.Stop()
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

	tc.sendMu.Lock()
	defer tc.sendMu.Unlock()

	var frame []byte
	if tc.kissFraming {
		frame = appendFrameKISS(tc.txFrame[:0], data)
	} else {
		frame = appendFrameHDLC(tc.txFrame[:0], data)
	}
	tc.txFrame = frame

	debug.Log(debug.DebugAll, "TCP interface writing to network", "name", tc.Name, "bytes", len(frame))

	tc.Mutex.RLock()
	conn := tc.conn
	tc.Mutex.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection closed")
	}

	_, err := conn.Write(frame)
	if err != nil {
		debug.Log(debug.DebugVerbose, "TCP interface write failed", "name", tc.Name, "error", err)
		tc.Mutex.Lock()
		tc.Online = false
		initiator := tc.initiator
		detached := tc.Detached
		tc.Mutex.Unlock()
		if initiator && !detached && tc.reconnect != nil {
			tc.teardownConn()
			tc.reconnect.notifyFailure()
		}
	}
	return err
}

func (tc *TCPClientInterface) Send(data []byte, address string) error {
	if err := common.RejectReceiveOnly(tc); err != nil {
		return err
	}
	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", tc.Name, "bytes", len(data), "address", address)
	}

	masked, err := common.ApplyIFACOutbound(tc, data)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to mask outgoing packet for IFAC", "name", tc.Name, "error", err)
		return err
	}

	if err := tc.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugVerbose, "Interface failed to send data", "name", tc.Name, "error", err)
		return err
	}

	tc.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (tc *TCPClientInterface) readLoop() {
	var feed func([]byte)
	// Decoder is local to this readLoop so overlapping reconnect loops cannot
	// race on a shared assembler (feed/reset across connections).
	if tc.kissFraming {
		decoder := newKISSStreamDecoder(tc.MTU, tc.handlePacket)
		feed = decoder.feed
	} else {
		decoder := newTCPHDLCStreamDecoder(tc.MTU, tc.handlePacket)
		feed = decoder.feed
	}
	n := streamReadSize(tc.MTU)
	if cap(tc.readBuf) < n {
		tc.readBuf = make([]byte, n)
	}
	buffer := tc.readBuf[:n]

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
		if n == 0 {
			if err != nil {
				tc.Mutex.Lock()
				tc.Online = false
				detached := tc.Detached
				initiator := tc.initiator
				tc.Mutex.Unlock()

				if initiator && !detached {
					tc.teardownConn()
					if tc.reconnect != nil {
						tc.reconnect.notifyFailure()
					}
				} else {
					tc.teardown()
				}
				return
			}
			continue
		}
		feed(buffer[:n])
		if err != nil {
			tc.Mutex.Lock()
			tc.Online = false
			detached := tc.Detached
			initiator := tc.initiator
			tc.Mutex.Unlock()

			if initiator && !detached {
				tc.teardownConn()
				if tc.reconnect != nil {
					tc.reconnect.notifyFailure()
				}
			} else {
				tc.teardown()
			}
			return
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

	if debug.Enabled(debug.DebugAll) {
		debug.Log(debug.DebugAll, "Received packet", "type", fmt.Sprintf("0x%02x", data[0]), "size", len(data))
	}

	tc.ProcessIncoming(data)
}

func (tc *TCPClientInterface) teardownConn() {
	tc.Mutex.Lock()
	if tc.conn != nil {
		_ = tc.conn.Close()
		tc.conn = nil
	}
	tc.Mutex.Unlock()
}

func (tc *TCPClientInterface) teardown() {
	tc.Online = false
	tc.In = false
	tc.Out = false
	tc.teardownConn()
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

// appendFrameHDLC appends a complete HDLC frame to dst and returns the slice.
// Reuse dst across calls via dst = appendFrameHDLC(dst[:0], payload) to avoid
// per-packet allocations when cap(dst) is large enough.
func appendFrameHDLC(dst []byte, payload []byte) []byte {
	dst = append(dst, HDLCFlag)
	for _, b := range payload {
		if b == HDLCFlag || b == HDLCEsc {
			dst = append(dst, HDLCEsc, b^HDLCEscMask)
		} else {
			dst = append(dst, b)
		}
	}
	return append(dst, HDLCFlag)
}

func escapeKISS(data []byte) []byte {
	escaped := make([]byte, 0, len(data)*2)
	for _, b := range data {
		switch b {
		case KISSFend:
			escaped = append(escaped, KISSFesc, KISSTFend)
		case KISSFesc:
			escaped = append(escaped, KISSFesc, KISSTFesc)
		default:
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

func (tc *TCPClientInterface) IsReconnecting() bool {
	if tc.reconnect == nil {
		return false
	}
	return tc.reconnect.isActive()
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
	return tc.conn != nil && tc.Online && !tc.IsReconnecting()
}

// TargetHost returns the configured dial host for initiator clients.
func (tc *TCPClientInterface) TargetHost() string {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.targetAddr
}

// TargetPort returns the configured dial port for initiator clients.
func (tc *TCPClientInterface) TargetPort() int {
	tc.Mutex.RLock()
	defer tc.Mutex.RUnlock()
	return tc.targetPort
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
	spawned     map[string]*TCPServerClientInterface
	spawnMu     sync.Mutex
	spawnHook   func(*TCPServerClientInterface)
	cfg         *common.InterfaceConfig
	listener    net.Listener
	bindAddr    string
	bindPort    int
	preferIPv6  bool
	kissFraming bool
	i2pTunneled bool
	done        chan struct{}
	stopOnce    sync.Once
	txFrame     []byte
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
			// Upstream assigns BITRATE_GUESS to TCP server interfaces; a
			// zero bitrate would make the announce queue never drain.
			Bitrate: BitrateGuess,
		},
		connections: make(map[string]net.Conn),
		spawned:     make(map[string]*TCPServerClientInterface),
		bindAddr:    bindAddr,
		bindPort:    bindPort,
		preferIPv6:  preferIPv6,
		kissFraming: kissFraming,
		i2pTunneled: i2pTunneled,
		done:        make(chan struct{}),
		txFrame:     make([]byte, 0, DefaultMTU*2+4),
	}
	ts.SetTunnelsAllowed(true)
	ts.SetLosslessMedium(true)

	return ts, nil
}

// EnableSpawnedClients switches the listener to Python's spawned-interface
// model: every accepted connection becomes its own interface object that the
// node registers with transport, while the listener itself stays
// receive-only for outbound floods (Python TCPServerInterface.OUT = False).
func (ts *TCPServerInterface) EnableSpawnedClients(cfg *common.InterfaceConfig, spawn func(*TCPServerClientInterface)) {
	ts.Mutex.Lock()
	defer ts.Mutex.Unlock()
	ts.cfg = cfg
	ts.spawnHook = spawn
}

// AllowsOutgoing keeps the listener receive-only once spawned-client mode is
// on (Python TCPServerInterface.OUT = False): outbound floods reach each
// connection through its own spawned interface, not a broadcast on the
// listener. Without a spawn hook the legacy broadcast behaviour is kept.
func (ts *TCPServerInterface) AllowsOutgoing() bool {
	ts.Mutex.RLock()
	defer ts.Mutex.RUnlock()
	return ts.spawnHook == nil && !ts.ReceiveOnly
}

// SpawnedInterfaces lists live per-connection children (Python
// spawned_interfaces, used by the detach walk).
func (ts *TCPServerInterface) SpawnedInterfaces() []Interface {
	ts.spawnMu.Lock()
	defer ts.spawnMu.Unlock()
	out := make([]Interface, 0, len(ts.spawned))
	for _, c := range ts.spawned {
		out = append(out, c)
	}
	return out
}

// InterfaceConfig supplies the listener's config so spawned clients inherit
// gravity, costs and hop limiter through InterfaceConfigProvider.
func (ts *TCPServerInterface) InterfaceConfig() *common.InterfaceConfig {
	return ts.cfg
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
		return fmt.Errorf("failed to start TCP server: %w", common.WrapListenError(err))
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
				select {
				case <-time.After(50 * time.Millisecond):
				case <-done:
					return
				}
				continue
			}

			d, release := protect.AdmitConn(ts.Name)
			if !d.Allow {
				_ = conn.Close()
				continue
			}
			go func(c net.Conn, rel func()) {
				defer rel()
				ts.handleConnection(c)
			}(conn, release)
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
	// Accepted sockets get no liveness probe by default. A silent peer would
	// otherwise pin a goroutine and read buffer forever.
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
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

	ts.Mutex.RLock()
	spawn := ts.spawnHook
	ts.Mutex.RUnlock()
	if spawn != nil {
		client := newTCPServerClient(conn, ts, addr)
		ts.spawnMu.Lock()
		ts.spawned[addr] = client
		ts.spawnMu.Unlock()
		spawn(client)
		client.readLoop()
		return
	}

	ts.readFramedLoop(conn)
}

func (ts *TCPServerInterface) readFramedLoop(conn net.Conn) {
	peerKey := conn.RemoteAddr().String()
	onFrame := func(data []byte) {
		ts.ProcessIncomingFrom(data, peerKey)
	}
	var feed func([]byte)
	if ts.kissFraming {
		decoder := newKISSStreamDecoder(ts.MTU, onFrame)
		feed = decoder.feed
	} else {
		decoder := newTCPHDLCStreamDecoder(ts.MTU, onFrame)
		feed = decoder.feed
	}
	buf := make([]byte, streamReadSize(ts.MTU))

	for {
		ts.Mutex.RLock()
		done := ts.done
		ts.Mutex.RUnlock()

		select {
		case <-done:
			return
		default:
		}

		n, err := conn.Read(buf)
		if n == 0 {
			if err != nil {
				return
			}
			continue
		}
		feed(buf[:n])
		if err != nil {
			return
		}
	}
}

func (ts *TCPServerInterface) ProcessOutgoing(data []byte) error {
	ts.Mutex.RLock()
	online := ts.Online
	spawned := ts.spawnHook != nil
	ts.Mutex.RUnlock()

	if !online {
		return fmt.Errorf("interface offline")
	}

	// Python TCPServerInterface.process_outgoing is a no-op: in spawned-client
	// mode every send goes through the per-connection spawned interface.
	if spawned {
		return nil
	}

	// Build into a fresh buffer per call: sharing ts.txFrame lets concurrent
	// senders interleave writes into one backing array and puts torn frames
	// on the wire.
	var frame []byte
	if ts.kissFraming {
		frame = appendFrameKISS(nil, data)
	} else {
		frame = appendFrameHDLC(nil, data)
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
	if err := common.RejectReceiveOnly(ts); err != nil {
		return err
	}
	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", ts.Name, "bytes", len(data), "address", address)
	}

	masked, err := common.ApplyIFACOutbound(ts, data)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to mask outgoing packet for IFAC", "name", ts.Name, "error", err)
		return err
	}

	if err := ts.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugVerbose, "Interface failed to send data", "name", ts.Name, "error", err)
		return err
	}

	ts.updateBandwidthStats(uint64(len(masked)))
	return nil
}

// EndpointStatus reports dial-health for this interface's remote endpoint:
// dial failures, flaps, and active quarantine remaining.
func (tc *TCPClientInterface) EndpointStatus() EndpointStatus {
	return tc.reconnect.endpointStatus()
}

// TCPServerClientInterface is spawned per accepted connection on a
// TCPServerInterface, matching Python's spawned TCPClientInterface: each
// socket carries its own registration, counters and tunnel-synthesis state,
// while the listener itself stays receive-only.
type TCPServerClientInterface struct {
	BaseInterface
	conn         net.Conn
	peerKey      string
	parent       *TCPServerInterface
	kissFraming  bool
	sendMu       sync.Mutex
	txFrame      []byte
	wantsTunnel  bool
	tunnelID     []byte
	onDisconnect func()
}

func newTCPServerClient(conn net.Conn, ts *TCPServerInterface, peerKey string) *TCPServerClientInterface {
	c := &TCPServerClientInterface{
		BaseInterface: BaseInterface{
			Name:    "Client on " + ts.Name + "@" + peerKey,
			Mode:    ts.Mode,
			Type:    common.IFTypeTCP,
			Online:  true,
			Enabled: true,
			MTU:     ts.MTU,
			Bitrate: ts.Bitrate,
			In:      true,
			Out:     true,
		},
		conn:        conn,
		peerKey:     peerKey,
		parent:      ts,
		kissFraming: ts.kissFraming,
		txFrame:     make([]byte, 0, DefaultMTU*2+4),
	}
	// Python copies the listener's routing and identity config onto the
	// spawned interface (mode, gravity, hop_limiter, costs, IFAC).
	ts.Mutex.RLock()
	c.Gravity = ts.Gravity
	c.RecursivePRs = ts.RecursivePRs
	c.AnnouncesFromInternal = ts.AnnouncesFromInternal
	c.AnnouncesToInternal = ts.AnnouncesToInternal
	c.DestinationCost = ts.DestinationCost
	c.DestinationCostSet = ts.DestinationCostSet
	c.PRCost = ts.PRCost
	c.PRCostSet = ts.PRCostSet
	c.IFACIdentity = ts.IFACIdentity
	c.deferInboundIFAC = ts.deferInboundIFAC
	c.announceCap = ts.announceCap
	ts.Mutex.RUnlock()
	if n, ok := ts.HopLimiter(); ok {
		c.SetHopLimiter(n)
	}
	c.SetTunnelsAllowed(true)
	c.SetLosslessMedium(true)
	return c
}

func (c *TCPServerClientInterface) String() string {
	return "TCPServerClientInterface[" + c.Name + "]"
}

// InterfaceHash feeds the tunnel id (Python interface.get_hash()).
func (c *TCPServerClientInterface) InterfaceHash() []byte {
	return InterfaceHashFromName(c.Name)
}

func (c *TCPServerClientInterface) WantsTunnel() bool     { return c.wantsTunnel }
func (c *TCPServerClientInterface) SetWantsTunnel(v bool) { c.wantsTunnel = v }

func (c *TCPServerClientInterface) TunnelID() []byte {
	c.Mutex.RLock()
	defer c.Mutex.RUnlock()
	return append([]byte(nil), c.tunnelID...)
}

func (c *TCPServerClientInterface) SetTunnelID(id []byte) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	c.tunnelID = append(c.tunnelID[:0], id...)
}

// InterfaceConfig makes the spawned conn inherit the listener's config
// (InterfaceConfigProvider): gravity, costs, hop limiter.
func (c *TCPServerClientInterface) InterfaceConfig() *common.InterfaceConfig {
	if c.parent == nil {
		return nil
	}
	return c.parent.cfg
}

// SpawnedParent returns the owning TCPServerInterface for counter
// propagation (Python parent_interface).
func (c *TCPServerClientInterface) SpawnedParent() common.NetworkInterface {
	return c.parent
}

func (c *TCPServerClientInterface) SetDisconnectHooks(onDisconnect func()) {
	c.onDisconnect = onDisconnect
}

func (c *TCPServerClientInterface) GetConn() net.Conn { return c.conn }

func (c *TCPServerClientInterface) readLoop() {
	onFrame := func(data []byte) { c.processIncoming(data) }
	var feed func([]byte)
	if c.kissFraming {
		decoder := newKISSStreamDecoder(c.MTU, onFrame)
		feed = decoder.feed
	} else {
		decoder := newTCPHDLCStreamDecoder(c.MTU, onFrame)
		feed = decoder.feed
	}
	buf := make([]byte, streamReadSize(c.MTU))
	for {
		n, err := c.conn.Read(buf)
		if n == 0 {
			if err != nil {
				break
			}
			continue
		}
		feed(buf[:n])
		if err != nil {
			break
		}
	}
	c.Mutex.Lock()
	c.Online = false
	c.Mutex.Unlock()
	if c.parent != nil {
		c.parent.spawnMu.Lock()
		delete(c.parent.spawned, c.peerKey)
		c.parent.spawnMu.Unlock()
	}
	if c.onDisconnect != nil {
		c.onDisconnect()
	}
}

// processIncoming mirrors BaseInterface.ProcessIncomingFrom but hands the
// packet callback the spawned client itself, not the embedded base, so the
// tunnel handler sees this connection's own synthesis state.
func (c *TCPServerClientInterface) processIncoming(data []byte) {
	c.Mutex.Lock()
	c.RxPackets++
	c.Mutex.Unlock()

	if !admitIncomingFrom(c, c.Name, data, c.peerKey) {
		return
	}

	c.Mutex.RLock()
	deferInbound := c.deferInboundIFAC
	c.Mutex.RUnlock()

	payload := data
	if !deferInbound {
		var ok bool
		payload, ok = common.ApplyIFACInbound(c, data)
		if !ok {
			debug.Log(debug.DebugVerbose, "Dropped packet failing IFAC policy", "name", c.Name, "size", len(data))
			return
		}
	}

	if cb := c.GetPacketCallback(); cb != nil {
		cb(payload, c)
	}
}

func (c *TCPServerClientInterface) ProcessIncoming(data []byte) {
	c.processIncoming(data)
}

func (c *TCPServerClientInterface) ProcessOutgoing(data []byte) error {
	c.Mutex.RLock()
	online := c.Online
	c.Mutex.RUnlock()
	if !online {
		return fmt.Errorf("interface offline")
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	var frame []byte
	if c.kissFraming {
		frame = appendFrameKISS(c.txFrame[:0], data)
	} else {
		frame = appendFrameHDLC(c.txFrame[:0], data)
	}
	c.txFrame = frame

	if _, err := c.conn.Write(frame); err != nil {
		debug.Log(debug.DebugVerbose, "TCP spawned client write failed", "name", c.Name, "error", err)
		return err
	}
	return nil
}

func (c *TCPServerClientInterface) Send(data []byte, address string) error {
	if err := common.RejectReceiveOnly(c); err != nil {
		return err
	}
	masked, err := common.ApplyIFACOutbound(c, data)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to mask outgoing packet for IFAC", "name", c.Name, "error", err)
		return err
	}
	if err := c.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugVerbose, "Interface failed to send data", "name", c.Name, "error", err)
		return err
	}
	c.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (c *TCPServerClientInterface) Detach() {
	c.Detached = true
	c.Online = false
	_ = c.conn.Close()
}

func (c *TCPServerClientInterface) Stop() error {
	c.Detach()
	return nil
}
