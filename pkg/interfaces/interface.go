// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
)

type Interface interface {
	GetName() string
	GetType() common.InterfaceType
	GetMode() common.InterfaceMode
	IsOnline() bool
	IsDetached() bool
	IsEnabled() bool
	Detach()
	Enable()
	Disable()
	Send(data []byte, addr string) error
	SetPacketCallback(common.PacketCallback)
	GetPacketCallback() common.PacketCallback
	ProcessIncoming([]byte)
	ProcessOutgoing([]byte) error
	SendPathRequest([]byte) error
	SendLinkPacket([]byte, []byte, time.Time) error
	Start() error
	Stop() error
	GetMTU() int
	GetConn() net.Conn
	GetBandwidthAvailable() bool
	common.NetworkInterface
}

type BaseInterface struct {
	Name      string
	Mode      common.InterfaceMode
	Type      common.InterfaceType
	Online    bool
	Enabled   bool
	Detached  bool
	In        bool
	Out       bool
	MTU       int
	Bitrate   int64
	TxBytes   uint64
	RxBytes   uint64
	TxPackets uint64
	RxPackets uint64
	lastTx    time.Time
	lastRx    time.Time

	Mutex          sync.RWMutex
	packetCallback common.PacketCallback

	// IFACIdentity is set when the interface participates in an IFAC network.
	// When non-nil, outbound packets are masked before transmit and inbound
	// packets are unmasked and verified; unauthenticated packets are dropped.
	IFACIdentity common.IFAC
}

func NewBaseInterface(name string, ifType common.InterfaceType, enabled bool) BaseInterface {
	return BaseInterface{
		Name:      name,
		Mode:      common.IFModeFull,
		Type:      ifType,
		Online:    false,
		Enabled:   enabled,
		Detached:  false,
		In:        false,
		Out:       false,
		MTU:       common.DefaultMTU,
		Bitrate:   BitrateMinimum,
		TxBytes:   0,
		RxBytes:   0,
		TxPackets: 0,
		RxPackets: 0,
		lastTx:    time.Now(),
		lastRx:    time.Now(),
	}
}

func (i *BaseInterface) SetPacketCallback(callback common.PacketCallback) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.packetCallback = callback
}

func (i *BaseInterface) GetPacketCallback() common.PacketCallback {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.packetCallback
}

// SetIFAC stores an Interface Access Code identity on this interface. Pass
// nil to disable IFAC. Subsequent Send / ProcessIncoming calls will use the
// new value.
func (i *BaseInterface) SetIFAC(id common.IFAC) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.IFACIdentity = id
}

// GetIFAC returns the configured Interface Access Code identity, or nil if
// IFAC is disabled.
func (i *BaseInterface) GetIFAC() common.IFAC {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.IFACIdentity
}

func (i *BaseInterface) ProcessIncoming(data []byte) {
	i.Mutex.Lock()
	i.RxBytes += uint64(len(data))
	i.RxPackets++
	i.Mutex.Unlock()

	stripped, ok := common.ApplyIFACInbound(i, data)
	if !ok {
		debug.Log(debug.DebugVerbose, "Dropped packet failing IFAC policy", "name", i.Name, "size", len(data))
		return
	}

	i.Mutex.RLock()
	callback := i.packetCallback
	i.Mutex.RUnlock()

	if callback != nil {
		callback(stripped, i)
	}
}

// ProcessOutgoing on the abstract BaseInterface is intentionally a fail-loud
// stub: any concrete network interface that uses BaseInterface as its base
// MUST override ProcessOutgoing to actually transmit bytes. Returning an
// error (and logging at CRITICAL) surfaces dynamic-dispatch mistakes
// (e.g. a *BaseInterface pointer leaking through a callback closure)
// instead of letting the transport silently swallow every outgoing packet.
func (i *BaseInterface) ProcessOutgoing(data []byte) error {
	debug.Log(debug.DebugCritical, "BaseInterface.ProcessOutgoing called directly; concrete interface type must override it", "name", i.Name, "bytes", len(data))
	return fmt.Errorf("ProcessOutgoing not implemented on abstract interfaces.BaseInterface (name=%q, %d bytes); concrete interface type must override it", i.Name, len(data))
}

func (i *BaseInterface) SendPathRequest(packet []byte) error {
	if !i.Online || i.Detached {
		return fmt.Errorf("interface offline or detached")
	}

	frame := make([]byte, 0, len(packet)+1)
	frame = append(frame, 0x01)
	frame = append(frame, packet...)

	return i.ProcessOutgoing(frame)
}

func (i *BaseInterface) SendLinkPacket(dest []byte, data []byte, timestamp time.Time) error {
	if !i.Online || i.Detached {
		return fmt.Errorf("interface offline or detached")
	}

	frame := make([]byte, 0, len(dest)+len(data)+9)
	frame = append(frame, 0x02)
	frame = append(frame, dest...)

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(timestamp.Unix())) // #nosec G115
	frame = append(frame, ts...)
	frame = append(frame, data...)

	return i.ProcessOutgoing(frame)
}

func (i *BaseInterface) Detach() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.Detached = true
	i.Online = false
}

func (i *BaseInterface) IsEnabled() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.Enabled && i.Online && !i.Detached
}

func (i *BaseInterface) Enable() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	prevState := i.Enabled
	i.Enabled = true
	i.Online = true

	debug.Log(debug.DebugInfo, "Interface state changed", "name", i.Name, "enabled_prev", prevState, "enabled", i.Enabled, "online_prev", !i.Online, "online", i.Online)
}

func (i *BaseInterface) Disable() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.Enabled = false
	i.Online = false
	debug.Log(debug.DebugError, "Interface disabled and offline", "name", i.Name)
}

func (i *BaseInterface) GetName() string {
	return i.Name
}

func (i *BaseInterface) GetType() common.InterfaceType {
	return i.Type
}

func (i *BaseInterface) GetMode() common.InterfaceMode {
	return i.Mode
}

func (i *BaseInterface) GetMTU() int {
	return i.MTU
}

func (i *BaseInterface) IsOnline() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.Online
}

func (i *BaseInterface) IsDetached() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.Detached
}

func (i *BaseInterface) GetTxBytes() uint64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.TxBytes
}

func (i *BaseInterface) GetRxBytes() uint64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.RxBytes
}

func (i *BaseInterface) GetTxPackets() uint64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.TxPackets
}

func (i *BaseInterface) GetRxPackets() uint64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.RxPackets
}

func (i *BaseInterface) Start() error {
	return nil
}

func (i *BaseInterface) Stop() error {
	return nil
}

func (i *BaseInterface) Send(data []byte, address string) error {
	debug.Log(debug.DebugVerbose, "Interface sending bytes", "name", i.Name, "bytes", len(data), "address", address)

	masked, err := common.ApplyIFACOutbound(i, data)
	if err != nil {
		debug.Log(debug.DebugCritical, "Failed to mask outgoing packet for IFAC", "name", i.Name, "error", err)
		return err
	}

	if err := i.ProcessOutgoing(masked); err != nil {
		debug.Log(debug.DebugCritical, "Interface failed to send data", "name", i.Name, "error", err)
		return err
	}

	i.updateBandwidthStats(uint64(len(masked)))
	return nil
}

func (i *BaseInterface) GetConn() net.Conn {
	return nil
}

func (i *BaseInterface) GetBandwidthAvailable() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()

	now := time.Now()
	timeSinceLastTx := now.Sub(i.lastTx)

	if timeSinceLastTx > time.Second {
		debug.Log(debug.DebugVerbose, "Interface bandwidth available", "name", i.Name, "idle_seconds", timeSinceLastTx.Seconds())
		return true
	}

	bytesPerSec := float64(i.TxBytes) / timeSinceLastTx.Seconds()
	currentUsage := bytesPerSec * 8
	maxUsage := float64(i.Bitrate) * PropagationRate

	available := currentUsage < maxUsage
	debug.Log(debug.DebugVerbose, "Interface bandwidth stats", "name", i.Name, "current_bps", currentUsage, "max_bps", maxUsage, "usage_percent", (currentUsage/maxUsage)*100, "available", available)

	return available
}

func (i *BaseInterface) updateBandwidthStats(bytes uint64) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	i.lastTx = time.Now()

	debug.Log(debug.DebugVerbose, "Interface updated bandwidth stats", "name", i.Name, "tx_bytes", i.TxBytes, "last_tx", i.lastTx)
}

type InterceptedInterface struct {
	Interface
	interceptor  func([]byte, common.NetworkInterface) error
	originalSend func([]byte, string) error
}

// Create constructor for intercepted interface
func NewInterceptedInterface(base Interface, interceptor func([]byte, common.NetworkInterface) error) *InterceptedInterface {
	return &InterceptedInterface{
		Interface:    base,
		interceptor:  interceptor,
		originalSend: base.Send,
	}
}

// Implement Send method for intercepted interface
func (i *InterceptedInterface) Send(data []byte, addr string) error {
	// Call interceptor if provided
	if i.interceptor != nil && len(data) > 0 {
		if err := i.interceptor(data, i); err != nil {
			debug.Log(debug.DebugError, "Failed to intercept outgoing packet", "error", err)
		}
	}

	// Call original send
	return i.originalSend(data, addr)
}
