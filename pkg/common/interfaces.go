// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package common

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
)

// IFAC is the subset of pkg/ifac.Identity that interfaces and transport need
// to authenticate and mask/unmask raw packets. Living in pkg/common avoids an
// import cycle (pkg/common -> pkg/ifac -> pkg/identity -> pkg/common).
type IFAC interface {
	// Size returns the per-interface IFAC size in bytes.
	Size() int
	// Mask wraps a raw outbound packet with an authenticated Interface Access
	// Code; the returned buffer is the bytes to write on the wire.
	Mask(raw []byte) ([]byte, error)
	// Unmask validates an inbound packet's IFAC. It returns (raw, true) when
	// the packet had a valid IFAC stripped, (raw, true) unchanged when the
	// IFAC flag is not set, and (nil, false) when validation failed.
	Unmask(raw []byte) ([]byte, bool, error)
}

// NetworkInterface defines the interface for all network communication methods
type NetworkInterface interface {
	// Core interface operations
	Start() error
	Stop() error
	Enable()
	Disable()
	Detach()

	// Network operations
	Send(data []byte, address string) error
	GetConn() net.Conn
	GetMTU() int
	GetName() string

	// Interface properties
	GetType() InterfaceType
	GetMode() InterfaceMode
	IsEnabled() bool
	IsOnline() bool
	IsDetached() bool
	GetBandwidthAvailable() bool

	// Packet handling
	ProcessIncoming([]byte)
	ProcessOutgoing([]byte) error
	SendPathRequest([]byte) error
	SendLinkPacket([]byte, []byte, time.Time) error
	SetPacketCallback(PacketCallback)
	GetPacketCallback() PacketCallback
	GetTxBytes() uint64
	GetRxBytes() uint64
	GetTxPackets() uint64
	GetRxPackets() uint64

	// Interface Access Code accessors. SetIFAC(nil) disables IFAC on this
	// interface. When IFAC is set, outbound packets are masked before send and
	// inbound packets without a valid IFAC are dropped, matching the policy of
	// Transport.transmit / Transport.inbound.
	SetIFAC(IFAC)
	GetIFAC() IFAC
}

// BaseInterface provides common implementation for network interfaces
type BaseInterface struct {
	Name     string
	Mode     InterfaceMode
	Type     InterfaceType
	Online   bool
	Enabled  bool
	Detached bool

	In  bool
	Out bool

	MTU     int
	Bitrate int64

	TxBytes   uint64
	RxBytes   uint64
	TxPackets uint64
	RxPackets uint64
	lastTx    time.Time

	Mutex          sync.RWMutex
	Owner          any
	PacketCallback PacketCallback

	// IFACIdentity is set when the interface participates in an IFAC network.
	IFACIdentity IFAC
}

// NewBaseInterface creates a new BaseInterface instance
func NewBaseInterface(name string, ifaceType InterfaceType, enabled bool) BaseInterface {
	return BaseInterface{
		Name:    name,
		Type:    ifaceType,
		Mode:    IFModeFull,
		Enabled: enabled,
		MTU:     DefaultMTU,
		Bitrate: BitrateMinimum,
		lastTx:  time.Now(),
	}
}

// Default implementations for BaseInterface
func (i *BaseInterface) GetType() InterfaceType {
	return i.Type
}

func (i *BaseInterface) GetMode() InterfaceMode {
	return i.Mode
}

func (i *BaseInterface) GetMTU() int {
	return i.MTU
}

func (i *BaseInterface) GetName() string {
	return i.Name
}

func (i *BaseInterface) IsEnabled() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.Enabled && i.Online && !i.Detached
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

func (i *BaseInterface) SetPacketCallback(callback PacketCallback) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.PacketCallback = callback
}

func (i *BaseInterface) GetPacketCallback() PacketCallback {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.PacketCallback
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

func (i *BaseInterface) Detach() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.Detached = true
	i.Online = false
}

func (i *BaseInterface) Enable() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.Enabled = true
	i.Online = true
}

func (i *BaseInterface) Disable() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.Enabled = false
	i.Online = false
}

// Default implementations that should be overridden by specific interfaces
func (i *BaseInterface) Start() error {
	return nil
}

func (i *BaseInterface) Stop() error {
	return nil
}

func (i *BaseInterface) GetConn() net.Conn {
	return nil
}

func (i *BaseInterface) Send(data []byte, address string) error {
	id := i.GetIFAC()
	if id != nil {
		masked, err := id.Mask(data)
		if err != nil {
			return err
		}
		data = masked
	}
	i.Mutex.Lock()
	i.TxBytes += uint64(len(data))
	i.TxPackets++
	i.lastTx = time.Now()
	i.Mutex.Unlock()
	return i.ProcessOutgoing(data)
}

func (i *BaseInterface) ProcessIncoming(data []byte) {
	i.Mutex.Lock()
	i.RxBytes += uint64(len(data))
	i.RxPackets++
	i.Mutex.Unlock()

	if id := i.GetIFAC(); id != nil {
		stripped, ok, _ := id.Unmask(data)
		if !ok || (len(data) >= 1 && data[0]&0x80 != 0x80) {
			return
		}
		data = stripped
	} else if len(data) >= 1 && data[0]&0x80 == 0x80 {
		return
	}

	if i.PacketCallback != nil {
		i.PacketCallback(data, i)
	}
}

// ProcessOutgoing on the abstract BaseInterface is intentionally a fail-loud
// stub: any concrete network interface that uses BaseInterface as its base
// MUST override ProcessOutgoing to actually transmit bytes. Returning an
// error here surfaces dynamic-dispatch mistakes (e.g. a *BaseInterface
// pointer leaking through a callback closure) instead of letting the
// transport silently swallow every outgoing packet.
func (i *BaseInterface) ProcessOutgoing(data []byte) error {
	return fmt.Errorf("ProcessOutgoing not implemented on abstract common.BaseInterface (name=%q, %d bytes); concrete interface type must override it", i.Name, len(data))
}

func (i *BaseInterface) SendPathRequest(data []byte) error {
	return i.Send(data, "")
}

func (i *BaseInterface) SendLinkPacket(dest []byte, data []byte, timestamp time.Time) error {
	// Create link packet
	packet := make([]byte, 0, len(dest)+len(data)+9) // 1 byte type + dest + 8 byte timestamp
	packet = append(packet, 0x02)                    // Link packet type
	packet = append(packet, dest...)

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(timestamp.Unix())) // #nosec G115
	packet = append(packet, ts...)

	packet = append(packet, data...)

	return i.Send(packet, "")
}

// SetIFAC stores an Interface Access Code identity on this interface. Pass nil
// to disable IFAC. Subsequent calls to ApplyIFACOutbound / ApplyIFACInbound
// will use the new value.
func (i *BaseInterface) SetIFAC(id IFAC) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.IFACIdentity = id
}

// GetIFAC returns the Interface Access Code identity, or nil if IFAC is not
// configured on this interface.
func (i *BaseInterface) GetIFAC() IFAC {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.IFACIdentity
}

// ApplyIFACOutbound masks raw with the interface's IFAC, if configured. When
// the interface has no IFAC the input is returned unchanged. Errors are
// returned to the caller; the caller should typically drop the packet on
// error.
func ApplyIFACOutbound(iface NetworkInterface, raw []byte) ([]byte, error) {
	if iface == nil {
		return raw, nil
	}
	id := iface.GetIFAC()
	if id == nil {
		return raw, nil
	}
	return id.Mask(raw)
}

// ApplyIFACInbound applies the IFAC policy of Transport.inbound. It
// returns the recovered raw packet plus a boolean that is true when the
// packet should continue through normal processing, and false when the
// packet must be dropped.
//
// Policy: if the interface has IFAC configured, the IFAC flag must be set
// AND the IFAC must verify, otherwise drop. If the interface has no IFAC
// configured, packets with the IFAC flag set are dropped.
func ApplyIFACInbound(iface NetworkInterface, raw []byte) ([]byte, bool) {
	if len(raw) < 2 {
		return raw, false
	}
	hasFlag := raw[0]&0x80 == 0x80
	var id IFAC
	if iface != nil {
		id = iface.GetIFAC()
	}
	if id == nil {
		if hasFlag {
			return nil, false
		}
		return raw, true
	}
	if !hasFlag {
		return nil, false
	}
	stripped, ok, err := id.Unmask(raw)
	if err != nil || !ok {
		return nil, false
	}
	return stripped, true
}

func (i *BaseInterface) GetBandwidthAvailable() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()

	// If no transmission in last second, bandwidth is available
	if time.Since(i.lastTx) > time.Second {
		return true
	}

	// Calculate current bandwidth usage
	bytesPerSec := float64(i.TxBytes) / time.Since(i.lastTx).Seconds()
	currentUsage := bytesPerSec * 8 // Convert to bits/sec

	// Check if usage is below threshold (2% of total bitrate)
	maxUsage := float64(i.Bitrate) * 0.02 // 2% propagation rate
	return currentUsage < maxUsage
}
