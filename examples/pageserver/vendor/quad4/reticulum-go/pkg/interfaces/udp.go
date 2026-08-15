// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"sync"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

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
	if maxReconnectTries > 0 {
		ui.initReconnectDriver()
	}

	return ui, nil
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
	if ui.targetAddr != nil {
		_ = conn.SetReadBuffer(1064)
		_ = conn.SetWriteBuffer(1064)
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
	stripped, ok := common.ApplyIFACInbound(ui, data)
	if !ok {
		return
	}
	if callback := ui.GetPacketCallback(); callback != nil {
		callback(stripped, ui)
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

	return nil
}

func (ui *UDPInterface) Send(data []byte, address string) error {
	if err := common.RejectReceiveOnly(ui); err != nil {
		return err
	}
	debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", ui.Name, "bytes", len(data), "address", address)

	masked, err := common.ApplyIFACOutbound(ui, data)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to mask outgoing packet for IFAC", "name", ui.Name, "error", err)
		return err
	}

	if err := ui.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugCritical, "Interface failed to send data", "name", ui.Name, "error", err)
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
	ui.Mutex.Unlock()

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

		n, _, err := conn.ReadFromUDP(buffer)
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

		ui.ProcessIncoming(buffer[:n])
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
