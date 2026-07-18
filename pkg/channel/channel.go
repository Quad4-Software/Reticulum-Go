// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
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

// ErrLinkNotReady is returned when a send is attempted on a non-ready outlet.
var ErrLinkNotReady = errors.New("link not ready")

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
	link            transport.LinkInterface
	sendMu          sync.Mutex
	mutex           sync.RWMutex
	txRing          []*Envelope
	window          int
	windowMax       int
	windowMin       int
	nextSequence    uint16
	maxTries        int
	messageHandlers []messageHandlerEntry
	nextHandlerID   int
	factories       map[uint16]MessageConstructor
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
	return &Channel{
		link:            link,
		messageHandlers: make([]messageHandlerEntry, InitialHandlerCapacity),
		factories:       make(map[uint16]MessageConstructor),
		mutex:           sync.RWMutex{},
		windowMax:       WindowMaxSlow,
		windowMin:       WindowMinSlow,
		window:          WindowInitial,
		maxTries:        DefaultMaxTries,
	}
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
	return c.registerMessageType(msgType, ctor, false)
}

// RegisterSystemMessageType registers a system message constructor (MSGTYPE >= 0xf000).
func (c *Channel) RegisterSystemMessageType(msgType uint16, ctor MessageConstructor) error {
	return c.registerMessageType(msgType, ctor, true)
}

func (c *Channel) registerMessageType(msgType uint16, ctor MessageConstructor, system bool) error {
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
func (c *Channel) Send(msg MessageBase) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if !outletReady(c.link.GetStatus()) {
		return ErrLinkNotReady
	}

	body, err := msg.Pack()
	if err != nil {
		return err
	}

	c.mutex.Lock()
	reserved := c.nextSequence
	c.nextSequence = uint16((uint32(reserved) + 1) % SeqModulus)
	c.mutex.Unlock()

	raw, err := packEnvelope(msg.GetType(), reserved, body)
	if err != nil {
		c.mutex.Lock()
		c.nextSequence = reserved
		c.mutex.Unlock()
		return err
	}

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
	c.mutex.Unlock()

	timeout := c.getPacketTimeout(env.Tries)
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
		timeout := c.getPacketTimeout(env.Tries)
		c.link.SetPacketTimeout(packet, c.handleTimeout, timeout)
		return
	}
}

// handleDelivered handles packet delivery confirmations
func (c *Channel) handleDelivered(packet any) {
	if packet == nil {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for i, env := range c.txRing {
		if env == nil || env.Packet == nil || env.Packet != packet {
			continue
		}
		c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
		releaseEnvelope(env)
		break
	}
}

func (c *Channel) getPacketTimeout(tries int) time.Duration {
	rtt := c.link.GetRTT()
	if rtt < RTTMinThreshold {
		rtt = RTTMinThreshold
	}

	timeout := math.Pow(TimeoutBaseMultiplier, float64(tries-1)) * rtt * TimeoutRingMultiplier * float64(len(c.txRing)+TimeoutRingOffset)
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
// Registered factories unpack into typed messages. Unknown types become GenericMessage.
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

	msgData := make([]byte, length)
	copy(msgData, data[ChannelHeaderSize:ChannelHeaderSize+int(length)])

	c.mutex.RLock()
	ctor := c.factories[msgType]
	handlers := make([]messageHandlerEntry, len(c.messageHandlers))
	copy(handlers, c.messageHandlers)
	c.mutex.RUnlock()

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

	for _, entry := range handlers {
		if entry.handler != nil {
			if entry.handler(msg) {
				break
			}
		}
	}

	return nil
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

// TxRingLen returns the number of outstanding envelopes (tests and diagnostics).
func (c *Channel) TxRingLen() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.txRing)
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
	return nil
}
