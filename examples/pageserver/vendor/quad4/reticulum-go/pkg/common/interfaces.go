// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/health"
)

// IFAC is the subset of pkg/ifac.Identity that interfaces and transport need
// to authenticate and mask/unmask raw packets. Living in pkg/common avoids an
// import cycle (pkg/common -> pkg/ifac -> pkg/identity -> pkg/common).
type IFAC interface {
	// Size returns the per-interface IFAC size in bytes.
	Size() int
	// Mask wraps a raw outbound packet with an authenticated Interface Access
	// Code. The returned buffer is the bytes to write on the wire.

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

	// Path request frequency tracking (ingress/egress burst control)
	ReceivedPathRequest()
	SentPathRequest()
	ShouldIngressLimitPR() bool
	ShouldEgressLimitPR() bool
	SetPRBurstConfig(icPrBurstFreqNew, icPrBurstFreq, ecPrFreq float64, egressControl bool)
	SetIngressControl(enabled bool)
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

	Mutex          sync.RWMutex // exported so concrete interfaces can lock with parent fields
	Owner          any
	PacketCallback PacketCallback

	// IFACIdentity is set when the interface participates in an IFAC network.
	IFACIdentity IFAC

	// RecursivePRs enables unknown-path discovery on this interface.
	RecursivePRs bool

	// AnnouncesFromInternal controls rebroadcast of announces learned via an
	// internal-mode next hop (default true).
	AnnouncesFromInternal bool

	// ReceiveOnly blocks transmit when true (Python outgoing = no).
	ReceiveOnly bool
}

// NewBaseInterface creates a BaseInterface value for embedding at construction.
// Prefer NewBaseInterfacePtr when holding a standalone *BaseInterface.
// Do not copy a BaseInterface after it has been used (Mutex must not be copied).
func NewBaseInterface(name string, ifaceType InterfaceType, enabled bool) BaseInterface {
	return BaseInterface{
		Name:                  name,
		Type:                  ifaceType,
		Mode:                  IFModeFull,
		Enabled:               enabled,
		MTU:                   DefaultMTU,
		Bitrate:               BitrateMinimum,
		lastTx:                time.Now(),
		AnnouncesFromInternal: true,
	}
}

// NewBaseInterfacePtr returns a heap-allocated BaseInterface with the same defaults
// as NewBaseInterface. Prefer this when storing a standalone interface pointer.
func NewBaseInterfacePtr(name string, ifaceType InterfaceType, enabled bool) *BaseInterface {
	b := NewBaseInterface(name, ifaceType, enabled)
	return &b
}

// Default implementations for BaseInterface
func (i *BaseInterface) GetType() InterfaceType {
	return i.Type
}

func (i *BaseInterface) GetMode() InterfaceMode {
	return i.Mode
}

// RecursivePRsEnabled reports whether unknown-path discovery is enabled.
func (i *BaseInterface) RecursivePRsEnabled() bool {
	return i.RecursivePRs
}

// AnnouncesFromInternalFlag reports whether announces from internal next hops
// may be rebroadcast (default true).
func (i *BaseInterface) AnnouncesFromInternalFlag() bool {
	return i.AnnouncesFromInternal
}

// AllowsOutgoing reports whether this interface may transmit (config OUT).
func (i *BaseInterface) AllowsOutgoing() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return !i.ReceiveOnly
}

// SetOutgoingAllowed sets the config-driven transmit permit.
func (i *BaseInterface) SetOutgoingAllowed(allowed bool) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.ReceiveOnly = !allowed
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
	if err := RejectReceiveOnly(i); err != nil {
		return err
	}
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
// returned to the caller. The caller should typically drop the packet on

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
	ifaceName := ""
	if iface != nil {
		ifaceName = iface.GetName()
	}
	if len(raw) < 2 {
		health.Inc(ifaceName, health.KindIFACFail)
		return raw, false
	}
	hasFlag := raw[0]&0x80 == 0x80
	var id IFAC
	if iface != nil {
		id = iface.GetIFAC()
	}
	if id == nil {
		if hasFlag {
			health.Inc(ifaceName, health.KindIFACFail)
			return nil, false
		}
		health.Inc(ifaceName, health.KindRxOK)
		return raw, true
	}
	if !hasFlag {
		health.Inc(ifaceName, health.KindIFACFail)
		return nil, false
	}
	stripped, ok, err := id.Unmask(raw)
	if err != nil || !ok {
		health.Inc(ifaceName, health.KindIFACFail)
		return nil, false
	}
	health.Inc(ifaceName, health.KindRxOK)
	return stripped, true
}

func (i *BaseInterface) GetBandwidthAvailable() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()

	// common.BaseInterface has no sampled TX rate. Never divide lifetime
	// TxBytes by elapsed (that falsely closes announce gates). Always allow.
	return true
}

// ReceivedPathRequest records an incoming path request.
func (i *BaseInterface) ReceivedPathRequest() {}

// SentPathRequest records an outgoing path request.
func (i *BaseInterface) SentPathRequest() {}

// ReceivedAnnounce records an incoming announce.
func (i *BaseInterface) ReceivedAnnounce() {}

// SentAnnounce records an outgoing announce.
func (i *BaseInterface) SentAnnounce() {}

// IncomingAnnounceFrequency returns the estimated incoming announce rate in Hz.
func (i *BaseInterface) IncomingAnnounceFrequency() float64 { return 0 }

// OutgoingAnnounceFrequency returns the estimated outgoing announce rate in Hz.
func (i *BaseInterface) OutgoingAnnounceFrequency() float64 { return 0 }

// IncomingPRFrequency returns the estimated incoming path-request rate in Hz.
func (i *BaseInterface) IncomingPRFrequency() float64 { return 0 }

// OutgoingPRFrequency returns the estimated outgoing path-request rate in Hz.
func (i *BaseInterface) OutgoingPRFrequency() float64 { return 0 }

// PRBurstActive reports whether path-request ingress burst limiting is active.
func (i *BaseInterface) PRBurstActive() bool { return false }

// SampleTraffic updates current RX/TX bitrates from byte-counter deltas.
func (i *BaseInterface) SampleTraffic() {}

// GetRxSpeed returns the most recently sampled receive bitrate in bits/sec.
func (i *BaseInterface) GetRxSpeed() float64 { return 0 }

// GetTxSpeed returns the most recently sampled transmit bitrate in bits/sec.
func (i *BaseInterface) GetTxSpeed() float64 { return 0 }

// ShouldIngressLimitPR reports whether ingress path-request limiting is active.
func (i *BaseInterface) ShouldIngressLimitPR() bool { return false }

// ShouldEgressLimitPR reports whether egress path-request limiting is active.
func (i *BaseInterface) ShouldEgressLimitPR() bool { return false }

// SetPRBurstConfig configures path-request burst thresholds.
func (i *BaseInterface) SetPRBurstConfig(_, _, _ float64, _ bool) {}

// SetIngressControl sets whether ingress limiting is enabled.
func (i *BaseInterface) SetIngressControl(_ bool) {}
