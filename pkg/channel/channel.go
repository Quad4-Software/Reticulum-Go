// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package channel

import (
	"errors"
	"math"
	"sync"
	"time"

	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// ErrLinkNotReady is returned when a send is attempted on a non-ready outlet.
var ErrLinkNotReady = errors.New("link not ready")

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

// Send transmits a message over the channel.
// Sequence allocation and tx-ring emplace happen only after a successful
// outlet send so a failing link cannot leave ghost envelopes or sequence holes.
func (c *Channel) Send(msg MessageBase) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if !outletReady(c.link.GetStatus()) {
		return ErrLinkNotReady
	}

	data, err := msg.Pack()
	if err != nil {
		return err
	}

	c.mutex.Lock()
	reserved := c.nextSequence
	c.nextSequence = (reserved + 1) % SeqModulus
	c.mutex.Unlock()

	packet := c.link.Send(data)
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
		Raw:       data,
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
// Each handler receives the same *GenericMessage, treat it as read-only unless the
// handler stops the chain (returns true). Data aliases the input slice.
func (c *Channel) HandleInbound(data []byte) error {
	if len(data) < ChannelHeaderSize {
		return errors.New("channel packet too short")
	}

	msgType := uint16(data[0])<<ChannelHeaderBits | uint16(data[1])
	sequence := uint16(data[2])<<ChannelHeaderBits | uint16(data[3])
	length := uint16(data[4])<<ChannelHeaderBits | uint16(data[5])

	if len(data) < ChannelHeaderSize+int(length) {
		return errors.New("channel packet incomplete")
	}

	msgData := data[ChannelHeaderSize : ChannelHeaderSize+length]

	c.mutex.Lock()
	defer c.mutex.Unlock()

	msg := GenericMessage{
		Type: msgType,
		Data: msgData,
		Seq:  sequence,
	}
	for _, entry := range c.messageHandlers {
		if entry.handler != nil {
			if entry.handler(&msg) {
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
