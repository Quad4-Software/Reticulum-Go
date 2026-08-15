// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// ErrLinkNotReady is returned when a send is attempted on a non-ready outlet
// or when the TX window is full, matching Python ChannelException ME_LINK_NOT_READY.
var ErrLinkNotReady = errors.New("link not ready")

// ErrTooBig is returned when the packed envelope exceeds the outlet MDU,
// matching Python ChannelException ME_TOO_BIG.
var ErrTooBig = errors.New("channel message too big")

// SystemMessageTypeMin is the lower bound for system-reserved MSGTYPE values.
// Matches Python RNS Channel (MSGTYPE >= 0xf000).
const SystemMessageTypeMin uint16 = 0xf000

var envelopePool = sync.Pool{
	New: func() any {
		return new(Envelope)
	},
}

func releaseEnvelope(env *Envelope) {
	if env == nil {
		return
	}
	*env = Envelope{}
	envelopePool.Put(env)
}

// MessageBase is the interface for messages sent over a Channel.
type MessageBase interface {
	Pack() ([]byte, error)
	Unpack([]byte) error
	GetType() uint16
}

// MessageConstructor builds an empty message for inbound unpacking.
type MessageConstructor func() MessageBase

// Channel provides reliable message delivery over a transport link.
// Sends reserve a sequence only after a successful outlet transmit, matching
// the Python 1.3.0 ghost-envelope fix while keeping a single-outlet model.
type Channel struct {
	link              transport.LinkInterface
	sendMu            sync.Mutex
	mutex             sync.RWMutex
	txRing            []*Envelope
	rxRing            []rxEnvelope
	window            int
	windowMax         int
	windowMin         int
	windowFlexibility int
	fastRateRounds    int
	mediumRateRounds  int
	nextSequence      uint16
	nextRxSequence    uint16
	maxTries          int
	messageHandlers   []messageHandlerEntry
	nextHandlerID     int
	factories         map[uint16]MessageConstructor
}

type rxEnvelope struct {
	sequence uint16
	message  MessageBase
}

type messageHandlerEntry struct {
	id      int
	handler func(MessageBase) bool
}

// Envelope wraps a message with metadata for transmission
type Envelope struct {
	Sequence  uint16
	Message   MessageBase
	Raw       []byte
	Packet    any
	Tries     int
	Timestamp time.Time
}

// NewChannel creates a new Channel for the given link.
func NewChannel(link transport.LinkInterface) *Channel {
	c := &Channel{
		link:              link,
		messageHandlers:   make([]messageHandlerEntry, InitialHandlerCapacity),
		factories:         make(map[uint16]MessageConstructor),
		mutex:             sync.RWMutex{},
		windowMax:         WindowMaxSlow,
		windowMin:         WindowMinSlow,
		window:            WindowInitial,
		windowFlexibility: WindowFlexibility,
		maxTries:          DefaultMaxTries,
	}
	if link != nil && link.GetRTT() > RTTSlow {
		c.window = 1
		c.windowMax = 1
		c.windowMin = 1
		c.windowFlexibility = 1
	}
	return c
}

// outletReady reports whether the link may accept channel traffic.
// Accepts both transport.StatusActive (wrappers and tests) and link ACTIVE
// (0x02) used by real pkg/link sessions.
func outletReady(status byte) bool {
	return status == transport.StatusActive || status == 0x02
}

// packetTransmitted reports whether outlet.Send produced a usable packet.
func packetTransmitted(pkt any) bool {
	if pkt == nil {
		return false
	}
	if p, ok := pkt.(*packet.Packet); ok {
		return p != nil && len(p.Raw) > 0
	}
	return true
}

// RegisterMessageType registers a user message constructor for inbound dispatch.
// Types >= 0xf000 are system-reserved and must use RegisterSystemMessageType.
func (c *Channel) RegisterMessageType(msgType uint16, ctor MessageConstructor) error {
	return c.bindMessageFactory(msgType, ctor, false)
}

// RegisterSystemMessageType registers a system message constructor (MSGTYPE >= 0xf000).
func (c *Channel) RegisterSystemMessageType(msgType uint16, ctor MessageConstructor) error {
	return c.bindMessageFactory(msgType, ctor, true)
}

func (c *Channel) bindMessageFactory(msgType uint16, ctor MessageConstructor, system bool) error {
	if ctor == nil {
		return errors.New("channel: nil message constructor")
	}
	if msgType >= SystemMessageTypeMin && !system {
		return fmt.Errorf("channel: MSGTYPE 0x%04x is system-reserved", msgType)
	}
	if msgType < SystemMessageTypeMin && system {
		return fmt.Errorf("channel: MSGTYPE 0x%04x is not a system type", msgType)
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.factories[msgType] = ctor
	return nil
}

// packEnvelope builds the Python-compatible wire envelope:
// big-endian MSGTYPE, sequence, length, then message body.
func packEnvelope(msgType, sequence uint16, body []byte) ([]byte, error) {
	if len(body) > 0xffff {
		return nil, fmt.Errorf("channel: message body too large (%d)", len(body))
	}
	raw := make([]byte, ChannelHeaderSize+len(body))
	binary.BigEndian.PutUint16(raw[0:2], msgType)
	binary.BigEndian.PutUint16(raw[2:4], sequence)
	binary.BigEndian.PutUint16(raw[4:6], uint16(len(body))) // #nosec G115 - length bounded above
	copy(raw[ChannelHeaderSize:], body)
	return raw, nil
}

// Send transmits a message over the channel.
// Sequence allocation and tx-ring emplace happen only after a successful
// outlet send so a failing link cannot leave ghost envelopes or sequence holes.
// A full TX window or packed envelope larger than the outlet MDU is refused,
// matching Python Channel.send.
func (c *Channel) Send(msg MessageBase) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if !c.IsReadyToSend() {
		return ErrLinkNotReady
	}

	body, err := msg.Pack()
	if err != nil {
		return err
	}

	c.mutex.Lock()
	reserved := c.nextSequence
	c.mutex.Unlock()

	raw, err := packEnvelope(msg.GetType(), reserved, body)
	if err != nil {
		return err
	}
	if len(raw) > c.outletMDU() {
		return ErrTooBig
	}

	c.mutex.Lock()
	if c.nextSequence != reserved {
		c.mutex.Unlock()
		return ErrLinkNotReady
	}
	c.nextSequence = uint16((uint32(reserved) + 1) % SeqModulus)
	c.mutex.Unlock()

	packet := c.link.Send(raw)
	if !packetTransmitted(packet) {
		c.mutex.Lock()
		c.nextSequence = reserved
		c.mutex.Unlock()
		return ErrLinkNotReady
	}

	env := envelopePool.Get().(*Envelope)
	*env = Envelope{
		Sequence:  reserved,
		Message:   msg,
		Raw:       raw,
		Packet:    packet,
		Tries:     1,
		Timestamp: time.Now(),
	}

	c.mutex.Lock()
	c.txRing = append(c.txRing, env)
	timeout := c.packetTimeoutLocked(env.Tries)
	c.mutex.Unlock()

	c.link.SetPacketTimeout(packet, c.handleTimeout, timeout)
	c.link.SetPacketDelivered(packet, c.handleDelivered)

	return nil
}

// handleTimeout handles packet timeout events
func (c *Channel) handleTimeout(packet any) {
	if packet == nil {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for i := 0; i < len(c.txRing); i++ {
		env := c.txRing[i]
		if env == nil || env.Packet == nil || env.Packet != packet {
			continue
		}
		c.shrinkWindowOnTimeoutLocked()
		if env.Tries >= c.maxTries {
			c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
			releaseEnvelope(env)
			return
		}
		env.Tries++
		if err := c.link.Resend(packet); err != nil {
			debug.Log(debug.DebugInfo, "Failed to resend packet", "error", err)
			c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
			releaseEnvelope(env)
			return
		}
		timeout := c.packetTimeoutLocked(env.Tries)
		c.link.SetPacketTimeout(packet, c.handleTimeout, timeout)
		return
	}
}

// handleDelivered handles packet delivery confirmations
func (c *Channel) handleDelivered(packet any) {
	if packet == nil {
		return
	}
	rtt := 0.0
	if c.link != nil {
		rtt = c.link.GetRTT()
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for i, env := range c.txRing {
		if env == nil || env.Packet == nil || env.Packet != packet {
			continue
		}
		c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
		releaseEnvelope(env)
		c.adjustWindowOnDeliveredLocked(rtt)
		break
	}
}

// packetTimeoutSeconds matches Python Channel._get_packet_timeout_time:
// pow(1.5, tries-1) * max(rtt*2.5, 0.025) * (tx_ring_len + 1.5).
func packetTimeoutSeconds(tries int, rtt float64, txRingLen int) float64 {
	if tries < 1 {
		tries = 1
	}
	if txRingLen < 0 {
		txRingLen = 0
	}
	return math.Pow(TimeoutBaseMultiplier, float64(tries-1)) * math.Max(rtt*TimeoutRingMultiplier, RTTMinThreshold) * (float64(txRingLen) + TimeoutRingOffset)
}

// packetTimeoutLocked computes the retry timeout. Caller must hold c.mutex.
func (c *Channel) packetTimeoutLocked(tries int) time.Duration {
	rtt := 0.0
	if c.link != nil {
		rtt = c.link.GetRTT()
	}
	timeout := packetTimeoutSeconds(tries, rtt, len(c.txRing))
	return time.Duration(timeout * float64(time.Second))
}

// AddMessageHandler registers a handler for inbound messages and returns its ID.
func (c *Channel) AddMessageHandler(handler func(MessageBase) bool) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	id := c.nextHandlerID
	c.nextHandlerID++
	c.messageHandlers = append(c.messageHandlers, messageHandlerEntry{id: id, handler: handler})
	return id
}

// RemoveMessageHandler unregisters the handler with the given ID.
func (c *Channel) RemoveMessageHandler(id int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for i, entry := range c.messageHandlers {
		if entry.id == id {
			c.messageHandlers = append(c.messageHandlers[:i], c.messageHandlers[i+1:]...)
			break
		}
	}
}

// HandleInbound processes an inbound channel packet and dispatches to registered handlers.
// Sequences are buffered on the RX ring and delivered in order, duplicates are
// dropped, matching Python Channel._receive.
func (c *Channel) HandleInbound(data []byte) error {
	if len(data) < ChannelHeaderSize {
		return errors.New("channel packet too short")
	}

	msgType := binary.BigEndian.Uint16(data[0:2])
	sequence := binary.BigEndian.Uint16(data[2:4])
	length := binary.BigEndian.Uint16(data[4:6])

	if len(data) < ChannelHeaderSize+int(length) {
		return errors.New("channel packet incomplete")
	}

	c.mutex.RLock()
	stale := staleRXSequence(sequence, c.nextRxSequence)
	ctor := c.factories[msgType]
	c.mutex.RUnlock()
	if stale {
		return nil
	}

	msgData := make([]byte, length)
	copy(msgData, data[ChannelHeaderSize:ChannelHeaderSize+int(length)])

	var msg MessageBase
	if ctor != nil {
		msg = ctor()
		if err := msg.Unpack(msgData); err != nil {
			return err
		}
	} else {
		msg = &GenericMessage{
			Type: msgType,
			Data: msgData,
			Seq:  sequence,
		}
	}

	c.mutex.Lock()
	if staleRXSequence(sequence, c.nextRxSequence) {
		c.mutex.Unlock()
		return nil
	}
	if !c.emplaceRXLocked(rxEnvelope{sequence: sequence, message: msg}) {
		c.mutex.Unlock()
		return nil
	}
	delivered := c.drainRXLocked()
	handlers := make([]messageHandlerEntry, len(c.messageHandlers))
	copy(handlers, c.messageHandlers)
	c.mutex.Unlock()

	for _, m := range delivered {
		for _, entry := range handlers {
			if entry.handler != nil && entry.handler(m) {
				break
			}
		}
	}

	return nil
}

// staleRXSequence reports whether seq is behind nextRx and outside the wrap
// window, matching Python Channel._receive WINDOW_MAX overflow logic.
func staleRXSequence(seq, next uint16) bool {
	if seq >= next {
		return false
	}
	windowOverflow := uint16((uint32(next) + uint32(WindowMax)) % SeqModulus)
	if windowOverflow < next {
		return seq > windowOverflow
	}
	return true
}

func (c *Channel) emplaceRXLocked(env rxEnvelope) bool {
	for i, existing := range c.rxRing {
		if env.sequence == existing.sequence {
			return false
		}
		dist := int32(c.nextRxSequence) - int32(env.sequence)
		if env.sequence < existing.sequence && dist <= int32(SeqMax)/2 {
			c.rxRing = append(c.rxRing, rxEnvelope{})
			copy(c.rxRing[i+1:], c.rxRing[i:])
			c.rxRing[i] = env
			return true
		}
	}
	c.rxRing = append(c.rxRing, env)
	return true
}

func (c *Channel) drainRXLocked() []MessageBase {
	var out []MessageBase
	for {
		found := -1
		for i, env := range c.rxRing {
			if env.sequence == c.nextRxSequence {
				found = i
				break
			}
		}
		if found < 0 {
			return out
		}
		env := c.rxRing[found]
		c.rxRing = append(c.rxRing[:found], c.rxRing[found+1:]...)
		out = append(out, env.message)
		c.nextRxSequence = uint16((uint32(c.nextRxSequence) + 1) % SeqModulus)
	}
}

// GenericMessage is a default message implementation with type, data, and sequence.
type GenericMessage struct {
	Type uint16
	Data []byte
	Seq  uint16
}

// Pack returns the message payload.
func (g *GenericMessage) Pack() ([]byte, error) {
	return g.Data, nil
}

// Unpack sets the message payload from data.
func (g *GenericMessage) Unpack(data []byte) error {
	g.Data = data
	return nil
}

// GetType returns the message type.
func (g *GenericMessage) GetType() uint16 {
	return g.Type
}

// IsReadyToSend reports whether the TX window has room, matching Python
// Channel.is_ready_to_send (outstanding envelopes < window).
func (c *Channel) IsReadyToSend() bool {
	if c.link == nil || !outletReady(c.link.GetStatus()) {
		return false
	}
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.txRing) < c.window
}

// WaitReady blocks until IsReadyToSend or ctx is done.
func (c *Channel) WaitReady(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if c.link != nil && !outletReady(c.link.GetStatus()) {
			return ErrLinkNotReady
		}
		if c.IsReadyToSend() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

type mduOutlet interface {
	GetMDU() int
}

func (c *Channel) outletMDU() int {
	mdu := DefaultOutletMDU
	if c.link != nil {
		if g, ok := c.link.(mduOutlet); ok {
			if n := g.GetMDU(); n > 0 {
				mdu = n
			}
		}
	}
	return mdu
}

// MDU is bytes available for a channel message body, matching Python
// Channel.mdu (outlet MDU minus 6-byte envelope header).
func (c *Channel) MDU() int {
	mdu := max(min(c.outletMDU()-ChannelHeaderSize, 0xFFFF), 1)
	return mdu
}

// Window is the current send window (tests and diagnostics).
func (c *Channel) Window() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.window
}

// WindowMax is the current maximum send window (tests and diagnostics).
func (c *Channel) WindowMax() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.windowMax
}

// Caller must hold c.mutex.
func (c *Channel) adjustWindowOnDeliveredLocked(rtt float64) {
	if c.window < c.windowMax {
		c.window++
	}
	if rtt == 0 {
		return
	}
	if rtt > RTTFast {
		c.fastRateRounds = 0
		if rtt > RTTMedium {
			c.mediumRateRounds = 0
			return
		}
		c.mediumRateRounds++
		if c.windowMax < WindowMaxMedium && c.mediumRateRounds == FastRateThreshold {
			c.windowMax = WindowMaxMedium
			c.windowMin = WindowMinMedium
		}
		return
	}
	c.fastRateRounds++
	if c.windowMax < WindowMaxFast && c.fastRateRounds == FastRateThreshold {
		c.windowMax = WindowMaxFast
		c.windowMin = WindowMinFast
	}
}

// Caller must hold c.mutex.
func (c *Channel) shrinkWindowOnTimeoutLocked() {
	if c.window > c.windowMin {
		c.window--
	}
	if c.windowMax > c.windowMin+c.windowFlexibility {
		c.windowMax--
	}
}

// TxRingLen returns the number of outstanding envelopes (tests and diagnostics).
func (c *Channel) TxRingLen() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.txRing)
}

// WaitTxIdle blocks until the TX ring is empty or timeout elapses.
// Returns true when idle (safe to tear down the link after a final send).
func (c *Channel) WaitTxIdle(timeout time.Duration) bool {
	if timeout <= 0 {
		return c.TxRingLen() == 0
	}
	deadline := time.Now().Add(timeout)
	for {
		if c.TxRingLen() == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return c.TxRingLen() == 0
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// NextSequence returns the next sequence that would be assigned (tests).
func (c *Channel) NextSequence() uint16 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.nextSequence
}

// Close releases channel resources.
func (c *Channel) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for _, env := range c.txRing {
		releaseEnvelope(env)
	}
	c.txRing = nil
	c.rxRing = nil
	return nil
}
