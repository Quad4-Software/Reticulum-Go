// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
)

// Interface is the package-local name for a network interface.
// It matches common.NetworkInterface so transport and config share one contract.
type Interface interface {
	common.NetworkInterface
}

const (
	prFreqSamples     = 48
	prMinFreqHz       = 0.1
	prFreqDecay       = 1.0 / prMinFreqHz // 10 seconds
	icDequeMinSample  = 2
	icBurstMinSamples = 6
	icPRBurstFreqNew  = 3.0
	icPRBurstFreq     = 8.0
	ecPRFreq          = 5.0
	icNewTime         = 2 * 60 * 60 // 2 hours in seconds
	icBurstHold       = 15
	icBurstPenalty    = 15
)

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

	Mutex          sync.RWMutex // exported so concrete interfaces can lock with parent fields
	packetCallback common.PacketCallback

	// IFACIdentity is set when the interface participates in an IFAC network.
	// When non-nil, outbound packets are masked before transmit and inbound
	// packets are unmasked and verified. Unauthenticated packets are dropped.

	IFACIdentity common.IFAC

	// RecursivePRs enables unknown-path discovery on this interface.
	RecursivePRs bool

	// AnnouncesFromInternal controls rebroadcast of announces learned via an
	// internal-mode next hop (default true).
	AnnouncesFromInternal bool

	// ReceiveOnly blocks transmit when true (Python outgoing = no).
	// Zero value is false so unset interfaces still transmit.
	ReceiveOnly bool

	// Path request frequency tracking (ingress/egress burst control)
	created            time.Time
	ipFreqDeque        []time.Time
	opFreqDeque        []time.Time
	iaFreqDeque        []time.Time
	oaFreqDeque        []time.Time
	icPRBurstActive    bool
	icPRBurstActivated time.Time
	ingressControl     bool
	egressControl      bool
	icPRBurstFreqNewV  float64
	icPRBurstFreqV     float64
	ecPRFreqV          float64

	currentRXS float64
	currentTXS float64
	sampleRXB  uint64
	sampleTXB  uint64
	sampleTS   time.Time
}

// NewBaseInterface creates a BaseInterface value for embedding at construction.
// Do not copy a BaseInterface after it has been used (Mutex must not be copied).
func NewBaseInterface(name string, ifType common.InterfaceType, enabled bool) BaseInterface {
	return BaseInterface{
		Name:                  name,
		Mode:                  common.IFModeFull,
		Type:                  ifType,
		Online:                false,
		Enabled:               enabled,
		Detached:              false,
		In:                    false,
		Out:                   false,
		MTU:                   common.DefaultMTU,
		Bitrate:               BitrateMinimum,
		TxBytes:               0,
		RxBytes:               0,
		created:               time.Now(),
		AnnouncesFromInternal: true,
		ingressControl:        true,
		icPRBurstFreqNewV:     icPRBurstFreqNew,
		icPRBurstFreqV:        icPRBurstFreq,
		ecPRFreqV:             ecPRFreq,
		TxPackets:             0,
		RxPackets:             0,
		lastTx:                time.Now(),
		lastRx:                time.Now(),
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
	debug.Log(debug.DebugCritical, "BaseInterface.ProcessOutgoing called directly, concrete interface type must override it", "name", i.Name, "bytes", len(data))
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
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
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
	if err := common.RejectReceiveOnly(i); err != nil {
		return err
	}
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

	elapsed := time.Since(i.lastTx)
	// Coarse clocks (notably Windows) can report elapsed <= 0 on the same
	// tick as lastTx. Still apply the sampled TX gate in that case.
	if i.Bitrate <= 0 || elapsed > time.Second {
		debug.Log(debug.DebugVerbose, "Interface bandwidth available", "name", i.Name, "idle_seconds", elapsed.Seconds())
		return true
	}

	maxUsage := float64(i.Bitrate) * PropagationRate
	// Use sampled TX bitrate from SampleTraffic. Lifetime TxBytes/elapsed
	// falsely reports multi-Gbps after a few KB and permanently closes the
	// announce forward gate under normal mesh load.
	if i.currentTXS <= 0 {
		debug.Log(debug.DebugVerbose, "Interface bandwidth available", "name", i.Name, "idle_seconds", elapsed.Seconds())
		return true
	}
	available := i.currentTXS < maxUsage
	debug.Log(debug.DebugVerbose, "Interface bandwidth stats", "name", i.Name, "current_bps", i.currentTXS, "max_bps", maxUsage, "usage_percent", (i.currentTXS/maxUsage)*100, "available", available)
	return available
}

func (i *BaseInterface) updateBandwidthStats(bytes uint64) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	i.TxBytes += bytes
	i.lastTx = time.Now()

	debug.Log(debug.DebugVerbose, "Interface updated bandwidth stats", "name", i.Name, "tx_bytes", i.TxBytes, "last_tx", i.lastTx)
}

// ReceivedPathRequest records an incoming path request for frequency tracking.
func (i *BaseInterface) ReceivedPathRequest() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.ipFreqDeque = append(i.ipFreqDeque, time.Now())
	if len(i.ipFreqDeque) > prFreqSamples {
		i.ipFreqDeque = i.ipFreqDeque[1:]
	}
}

// SentPathRequest records an outgoing path request for frequency tracking.
func (i *BaseInterface) SentPathRequest() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.opFreqDeque = append(i.opFreqDeque, time.Now())
	if len(i.opFreqDeque) > prFreqSamples {
		i.opFreqDeque = i.opFreqDeque[1:]
	}
}

// ReceivedAnnounce records an incoming announce for frequency tracking.
func (i *BaseInterface) ReceivedAnnounce() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.iaFreqDeque = append(i.iaFreqDeque, time.Now())
	if len(i.iaFreqDeque) > prFreqSamples {
		i.iaFreqDeque = i.iaFreqDeque[1:]
	}
}

// SentAnnounce records an outgoing announce for frequency tracking.
func (i *BaseInterface) SentAnnounce() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.oaFreqDeque = append(i.oaFreqDeque, time.Now())
	if len(i.oaFreqDeque) > prFreqSamples {
		i.oaFreqDeque = i.oaFreqDeque[1:]
	}
}

// IncomingAnnounceFrequency returns the estimated incoming announce rate in Hz.
func (i *BaseInterface) IncomingAnnounceFrequency() float64 {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	return i.incomingAnnounceFrequency()
}

// OutgoingAnnounceFrequency returns the estimated outgoing announce rate in Hz.
func (i *BaseInterface) OutgoingAnnounceFrequency() float64 {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	return i.outgoingAnnounceFrequency()
}

// IncomingPRFrequency returns the estimated incoming path-request rate in Hz.
func (i *BaseInterface) IncomingPRFrequency() float64 {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	return i.incomingPRFrequency()
}

// OutgoingPRFrequency returns the estimated outgoing path-request rate in Hz.
func (i *BaseInterface) OutgoingPRFrequency() float64 {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	return i.outgoingPRFrequency()
}

// PRBurstActive reports whether path-request ingress burst limiting is active.
func (i *BaseInterface) PRBurstActive() bool {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.icPRBurstActive
}

// SampleTraffic updates current RX/TX bitrates from byte-counter deltas.
func (i *BaseInterface) SampleTraffic() {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	now := time.Now()
	if i.sampleTS.IsZero() {
		i.sampleRXB = i.RxBytes
		i.sampleTXB = i.TxBytes
		i.sampleTS = now
		return
	}
	elapsed := now.Sub(i.sampleTS).Seconds()
	if elapsed <= 0 {
		return
	}
	rxDiff := i.RxBytes - i.sampleRXB
	txDiff := i.TxBytes - i.sampleTXB
	i.currentRXS = float64(rxDiff*8) / elapsed
	i.currentTXS = float64(txDiff*8) / elapsed
	i.sampleRXB = i.RxBytes
	i.sampleTXB = i.TxBytes
	i.sampleTS = now
}

// GetRxSpeed returns the most recently sampled receive bitrate in bits/sec.
func (i *BaseInterface) GetRxSpeed() float64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentRXS
}

// GetTxSpeed returns the most recently sampled transmit bitrate in bits/sec.
func (i *BaseInterface) GetTxSpeed() float64 {
	i.Mutex.RLock()
	defer i.Mutex.RUnlock()
	return i.currentTXS
}

// SetPRBurstConfig configures path-request burst thresholds.
func (i *BaseInterface) SetPRBurstConfig(icPrBurstFreqNew, icPrBurstFreq, ecPrFreq float64, egressControl bool) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.icPRBurstFreqNewV = icPrBurstFreqNew
	i.icPRBurstFreqV = icPrBurstFreq
	i.ecPRFreqV = ecPrFreq
	i.egressControl = egressControl
}

// SetIngressControl sets whether ingress limiting is enabled.
func (i *BaseInterface) SetIngressControl(enabled bool) {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()
	i.ingressControl = enabled
}

func (i *BaseInterface) incomingAnnounceFrequency() float64 {
	n := len(i.iaFreqDeque)
	if n <= icDequeMinSample {
		return 0
	}
	oldest := i.iaFreqDeque[0]
	span := time.Since(oldest).Seconds()
	if span > prFreqDecay {
		i.iaFreqDeque = i.iaFreqDeque[1:]
	}
	if span <= 0 {
		return 0
	}
	return float64(n) / span
}

func (i *BaseInterface) outgoingAnnounceFrequency() float64 {
	n := len(i.oaFreqDeque)
	if n <= 1 {
		return 0
	}
	oldest := i.oaFreqDeque[0]
	span := time.Since(oldest).Seconds()
	if span > prFreqDecay {
		i.oaFreqDeque = i.oaFreqDeque[1:]
	}
	if span <= 0 {
		return 0
	}
	return float64(n) / span
}

func (i *BaseInterface) incomingPRFrequency() float64 {
	n := len(i.ipFreqDeque)
	if n <= icDequeMinSample {
		return 0
	}
	oldest := i.ipFreqDeque[0]
	span := time.Since(oldest).Seconds()
	if span > prFreqDecay {
		i.ipFreqDeque = i.ipFreqDeque[1:]
	}
	if span <= 0 {
		return 0
	}
	return float64(n) / span
}

func (i *BaseInterface) outgoingPRFrequency() float64 {
	n := len(i.opFreqDeque)
	if n <= 1 {
		return 0
	}
	oldest := i.opFreqDeque[0]
	span := time.Since(oldest).Seconds()
	if span > prFreqDecay {
		i.opFreqDeque = i.opFreqDeque[1:]
	}
	if span <= 0 {
		return 0
	}
	return float64(n) / span
}

func (i *BaseInterface) ShouldIngressLimitPR() bool {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	if !i.ingressControl {
		return false
	}

	freqThreshold := i.icPRBurstFreqV
	if time.Since(i.created).Seconds() < icNewTime {
		freqThreshold = i.icPRBurstFreqNewV
	}
	ipFreq := i.incomingPRFrequency()

	if i.icPRBurstActive {
		if ipFreq < freqThreshold && time.Since(i.icPRBurstActivated).Seconds() > icBurstHold {
			i.icPRBurstActive = false
		}
		return true
	}

	if ipFreq > freqThreshold {
		i.icPRBurstActive = true
		i.icPRBurstActivated = time.Now()
		return true
	}
	return false
}

func (i *BaseInterface) ShouldEgressLimitPR() bool {
	i.Mutex.Lock()
	defer i.Mutex.Unlock()

	if !i.egressControl {
		return false
	}

	opFreq := i.outgoingPRFrequency()
	if opFreq > i.ecPRFreqV {
		if len(i.opFreqDeque) >= icBurstMinSamples {
			return true
		}
	}
	return false
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
