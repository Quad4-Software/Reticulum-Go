// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/debug"
)

// udpKeepalivePayload is a one-byte datagram that deliberately fails packet
// decode on the remote side. It exists only to refresh NAT/firewall state
// toward the configured target (WireGuard PersistentKeepalive pattern) and
// is dropped by both Python and Go peers before reaching transport.
var udpKeepalivePayload = []byte{0x00}

type UDPInterface struct {
	BaseInterface
	conn              *net.UDPConn
	addr              *net.UDPAddr
	targetAddr        *net.UDPAddr
	readBuffer        []byte
	maxReconnectTries int
	reconnect         *reconnectDriver
	onDown            func()
	onUp              func()
	done              chan struct{}
	stopOnce          sync.Once
	keepalive         time.Duration
	lastWriteNs       atomic.Int64
}

func NewUDPInterface(name string, addr string, target string, enabled bool) (*UDPInterface, error) {
	return NewUDPInterfaceWithRetries(name, addr, target, enabled, 0)
}

func NewUDPInterfaceWithRetries(name string, addr string, target string, enabled bool, maxReconnectTries int) (*UDPInterface, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	var targetAddr *net.UDPAddr
	if target != "" {
		targetAddr, err = net.ResolveUDPAddr("udp", target)
		if err != nil {
			return nil, err
		}
	}

	ui := &UDPInterface{
		BaseInterface:     NewBaseInterface(name, common.IFTypeUDP, enabled),
		addr:              udpAddr,
		targetAddr:        targetAddr,
		readBuffer:        make([]byte, 1064),
		maxReconnectTries: maxReconnectTries,
		done:              make(chan struct{}),
	}

	ui.MTU = 1064
	ui.Bitrate = BitrateGuess
	if maxReconnectTries > 0 {
		ui.initReconnectDriver()
	}

	return ui, nil
}

// SetKeepaliveInterval enables the persistent keepalive loop. The interface
// sends a minimal hold-open datagram to the configured target whenever no
// outbound packet has been written for the interval, keeping NAT and
// stateful-firewall mappings alive. Zero disables it (default).
func (ui *UDPInterface) SetKeepaliveInterval(d time.Duration) {
	ui.Mutex.Lock()
	ui.keepalive = d
	ui.Mutex.Unlock()
}

func (ui *UDPInterface) SetConnectivityHooks(onDown, onUp func()) {
	ui.Mutex.Lock()
	ui.onDown = onDown
	ui.onUp = onUp
	ui.Mutex.Unlock()
}

func (ui *UDPInterface) initReconnectDriver() {
	ui.reconnect = newReconnectDriver(ui.Name, ui.maxReconnectTries, ui.done, ui.dialUDP, func(conn net.Conn) {
		udpConn, ok := conn.(*net.UDPConn)
		if !ok {
			return
		}
		if !ui.adoptConn(udpConn) {
			_ = udpConn.Close()
			return
		}
		ui.Mutex.RLock()
		onUp := ui.onUp
		ui.Mutex.RUnlock()
		if onUp != nil {
			onUp()
		}
		go ui.readLoop()
	})
}

func (ui *UDPInterface) adoptConn(conn *net.UDPConn) bool {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	if ui.Detached {
		return false
	}
	select {
	case <-ui.done:
		return false
	default:
	}
	ui.conn = conn
	ui.Online = true
	return true
}

func (ui *UDPInterface) dialUDP() (net.Conn, error) {
	conn, err := net.ListenUDP("udp", ui.addr)
	if err != nil {
		return nil, common.WrapListenError(err)
	}
	return conn, nil
}

func (ui *UDPInterface) GetName() string {
	return ui.Name
}

func (ui *UDPInterface) GetType() common.InterfaceType {
	return ui.Type
}

func (ui *UDPInterface) GetMode() common.InterfaceMode {
	return ui.Mode
}

func (ui *UDPInterface) IsOnline() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Online
}

func (ui *UDPInterface) IsDetached() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Detached
}

func (ui *UDPInterface) Detach() {
	ui.Mutex.Lock()
	ui.Detached = true
	ui.Online = false
	if ui.conn != nil {
		_ = ui.conn.Close()
		ui.conn = nil
	}
	ui.Mutex.Unlock()
	ui.stopOnce.Do(func() {
		if ui.done != nil {
			close(ui.done)
		}
	})
}

func (ui *UDPInterface) SetPacketCallback(callback common.PacketCallback) {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.packetCallback = callback
}

func (ui *UDPInterface) GetPacketCallback() common.PacketCallback {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.packetCallback
}

func (ui *UDPInterface) ProcessIncoming(data []byte) {
	ui.ProcessIncomingFromAddr(data, "")
}

// ProcessIncomingFromAddr is ProcessIncoming plus an optional remote address
// string. A UDP socket is commonly shared by many remote senders, so this
// gives each sender its own fair-share sub-bucket instead of letting one
// flooding peer exhaust the whole interface budget and cool down every
// other peer using the same socket. See admitIncomingFrom.
func (ui *UDPInterface) ProcessIncomingFromAddr(data []byte, peerKey string) {
	ui.Mutex.Lock()
	ui.RxBytes += uint64(len(data))
	ui.RxPackets++
	name := ui.Name
	ui.Mutex.Unlock()

	if !admitIncomingFrom(ui, name, data, peerKey) {
		return
	}

	// When registered with transport, IFAC is applied once in
	// preprocessInboundPacket (RNS 1.5.0). Applying it here too would
	// strip the IFAC flag and make transport treat a valid packet as a
	// missing-IFAC violation.
	payload := data
	if !ui.DeferInboundIFAC() {
		var ok bool
		payload, ok = common.ApplyIFACInbound(ui, data)
		if !ok {
			return
		}
	}
	if callback := ui.GetPacketCallback(); callback != nil {
		callback(payload, ui)
	}
}

func (ui *UDPInterface) ProcessOutgoing(data []byte) error {
	if !ui.IsOnline() {
		return fmt.Errorf("interface offline")
	}

	if ui.targetAddr == nil {
		return fmt.Errorf("no target address configured")
	}

	ui.Mutex.RLock()
	conn := ui.conn
	target := ui.targetAddr
	ui.Mutex.RUnlock()
	if conn == nil {
		return fmt.Errorf("connection closed")
	}

	_, err := conn.WriteToUDP(data, target)
	if err != nil {
		return fmt.Errorf("UDP write failed: %w", err)
	}
	ui.lastWriteNs.Store(time.Now().UnixNano())

	return nil
}

// keepaliveLoop emits a hold-open datagram when the link has been idle for
// one keepalive interval. Runs for the lifetime of a Start() generation and
// exits on done.
func (ui *UDPInterface) keepaliveLoop(done <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}

		ui.Mutex.RLock()
		conn := ui.conn
		target := ui.targetAddr
		online := ui.Online && !ui.Detached && ui.keepalive > 0
		ui.Mutex.RUnlock()
		if !online || conn == nil || target == nil {
			continue
		}
		if last := ui.lastWriteNs.Load(); last != 0 && time.Since(time.Unix(0, last)) < interval {
			continue
		}
		if _, err := conn.WriteToUDP(udpKeepalivePayload, target); err != nil {
			debug.Log(debug.DebugVerbose, "UDP keepalive write failed", "name", ui.Name, "error", err)
			continue
		}
		ui.updateBandwidthStats(uint64(len(udpKeepalivePayload)))
	}
}

func (ui *UDPInterface) Send(data []byte, address string) error {
	if err := common.RejectReceiveOnly(ui); err != nil {
		return err
	}
	if debug.Enabled(debug.DebugVerbose) {
		debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", ui.Name, "bytes", len(data), "address", address)
	}

	masked, err := common.ApplyIFACOutbound(ui, data)
	if err != nil {
		debug.Log(debug.DebugError, "Failed to mask outgoing packet for IFAC", "name", ui.Name, "error", err)
		return err
	}

	if err := ui.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugVerbose, "Interface failed to send data", "name", ui.Name, "error", err)
		return err
	}

	ui.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (ui *UDPInterface) GetConn() net.Conn {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.conn
}

func (ui *UDPInterface) GetTxBytes() uint64 {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.TxBytes
}

func (ui *UDPInterface) GetRxBytes() uint64 {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.RxBytes
}

func (ui *UDPInterface) GetMTU() int {
	return ui.MTU
}

func (ui *UDPInterface) GetBitrate() int {
	return int(ui.Bitrate)
}

func (ui *UDPInterface) Enable() {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Online = true
}

func (ui *UDPInterface) Disable() {
	ui.Mutex.Lock()
	defer ui.Mutex.Unlock()
	ui.Online = false
}

func (ui *UDPInterface) Start() error {
	ui.Mutex.Lock()
	if ui.conn != nil {
		ui.Mutex.Unlock()
		return fmt.Errorf("UDP interface already started")
	}
	select {
	case <-ui.done:
		ui.done = make(chan struct{})
		ui.stopOnce = sync.Once{}
	default:
		if ui.done == nil {
			ui.done = make(chan struct{})
			ui.stopOnce = sync.Once{}
		}
	}
	useReconnect := ui.maxReconnectTries > 0
	keepalive := ui.keepalive
	done := ui.done
	ui.Mutex.Unlock()

	if keepalive > 0 {
		go ui.keepaliveLoop(done, keepalive)
	}

	if useReconnect {
		ui.initReconnectDriver()
		ui.reconnect.start()
		return nil
	}

	conn, err := ui.dialUDP()
	if err != nil {
		return err
	}
	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		_ = conn.Close()
		return fmt.Errorf("unexpected UDP connection type")
	}
	if !ui.adoptConn(udpConn) {
		_ = conn.Close()
		return fmt.Errorf("failed to adopt UDP connection")
	}
	go ui.readLoop()
	return nil
}

func (ui *UDPInterface) Stop() error {
	ui.Detach()
	return nil
}

func (ui *UDPInterface) readLoop() {
	buffer := make([]byte, 1064)
	for {
		ui.Mutex.RLock()
		online := ui.Online
		detached := ui.Detached
		conn := ui.conn
		done := ui.done
		ui.Mutex.RUnlock()

		if !online || detached || conn == nil {
			return
		}

		select {
		case <-done:
			return
		default:
		}

		n, from, err := conn.ReadFromUDP(buffer)
		if err != nil {
			ui.Mutex.RLock()
			stillOnline := ui.Online
			detached := ui.Detached
			ui.Mutex.RUnlock()
			if stillOnline && !detached {
				debug.Log(debug.DebugError, "Error reading from UDP interface", "name", ui.Name, "error", err)
				ui.closeConn()
				ui.Mutex.RLock()
				onDown := ui.onDown
				ui.Mutex.RUnlock()
				if onDown != nil {
					onDown()
				}
				if ui.reconnect != nil {
					ui.reconnect.notifyFailure()
				}
			}
			return
		}

		peerKey := ""
		if from != nil {
			peerKey = from.String()
		}
		ui.ProcessIncomingFromAddr(buffer[:n], peerKey)
	}
}

func (ui *UDPInterface) closeConn() {
	ui.Mutex.Lock()
	if ui.conn != nil {
		_ = ui.conn.Close()
		ui.conn = nil
	}
	ui.Online = false
	ui.Mutex.Unlock()
}

func (ui *UDPInterface) IsEnabled() bool {
	ui.Mutex.RLock()
	defer ui.Mutex.RUnlock()
	return ui.Enabled && ui.Online && !ui.Detached
}
