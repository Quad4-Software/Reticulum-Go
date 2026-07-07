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
		maxReconnectTries: NormalizeMaxReconnectTries(maxReconnectTries),
		done:              make(chan struct{}),
	}

	ui.MTU = 1064
	ui.initReconnectDriver()

	return ui, nil
}

func (ui *UDPInterface) SetConnectivityHooks(onDown, onUp func()) {
	ui.onDown = onDown
	ui.onUp = onUp
}

func (ui *UDPInterface) initReconnectDriver() {
	ui.reconnect = newReconnectDriver(ui.Name, ui.maxReconnectTries, ui.done, ui.dialUDP, func(conn net.Conn) {
		udpConn, ok := conn.(*net.UDPConn)
		if !ok {
			return
		}
		ui.Mutex.Lock()
		ui.conn = udpConn
		ui.Online = true
		ui.Mutex.Unlock()
		if ui.onUp != nil {
			ui.onUp()
		}
		go ui.readLoop()
	})
}

func (ui *UDPInterface) dialUDP() (net.Conn, error) {
	conn, err := net.ListenUDP("udp", ui.addr)
	if err != nil {
		return nil, err
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
	defer ui.Mutex.Unlock()
	ui.Detached = true
	ui.Online = false
	if ui.conn != nil {
		ui.conn.Close() // #nosec G104
	}
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

	_, err := ui.conn.WriteToUDP(data, ui.targetAddr)
	if err != nil {
		return fmt.Errorf("UDP write failed: %v", err)
	}

	return nil
}

func (ui *UDPInterface) Send(data []byte, address string) error {
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
	// Only recreate done if it's nil or was closed
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
	ui.Mutex.Unlock()

	ui.initReconnectDriver()
	ui.reconnect.start()
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

		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			ui.Mutex.RLock()
			stillOnline := ui.Online
			detached := ui.Detached
			ui.Mutex.RUnlock()
			if stillOnline && !detached {
				debug.Log(debug.DebugError, "Error reading from UDP interface", "name", ui.Name, "error", err)
				ui.closeConn()
				if ui.onDown != nil {
					ui.onDown()
				}
				if ui.reconnect != nil {
					ui.reconnect.notifyFailure()
				}
			}
			return
		}

		ui.Mutex.Lock()
		// Auto-discover target address from first packet if not set
		if ui.targetAddr == nil {
			debug.Log(debug.DebugAll, "UDP interface discovered peer", "name", ui.Name, "peer", remoteAddr.String())
			ui.targetAddr = remoteAddr
		}
		ui.Mutex.Unlock()

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
