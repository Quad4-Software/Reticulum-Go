// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package channel

import (
	"errors"
	"math"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

// MessageBase is the interface for messages sent over a Channel.
type MessageBase interface {
	Pack() ([]byte, error)
	Unpack([]byte) error
	GetType() uint16
}

// Channel provides reliable message delivery over a transport link.
type Channel struct {
	link            transport.LinkInterface
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

// Send transmits a message over the channel
func (c *Channel) Send(msg MessageBase) error {
	if c.link.GetStatus() != transport.StatusActive {
		return errors.New("link not ready")
	}

	env := &Envelope{
		Sequence:  c.nextSequence,
		Message:   msg,
		Timestamp: time.Now(),
	}

	c.mutex.Lock()
	c.nextSequence = (c.nextSequence + 1) % SeqModulus
	c.txRing = append(c.txRing, env)
	c.mutex.Unlock()

	data, err := msg.Pack()
	if err != nil {
		return err
	}

	env.Raw = data
	packet := c.link.Send(data)
	env.Packet = packet
	env.Tries++

	timeout := c.getPacketTimeout(env.Tries)
	c.link.SetPacketTimeout(packet, c.handleTimeout, timeout)
	c.link.SetPacketDelivered(packet, c.handleDelivered)

	return nil
}

// handleTimeout handles packet timeout events
func (c *Channel) handleTimeout(packet any) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, env := range c.txRing {
		if env.Packet == packet {
			if env.Tries >= c.maxTries {
				// Remove from ring and notify failure
				return
			}
			env.Tries++
			if err := c.link.Resend(packet); err != nil { // #nosec G104
				// Handle resend error, e.g., log it or mark envelope as failed
				debug.Log(debug.DebugInfo, "Failed to resend packet", "error", err)
				// Optionally, mark the envelope as failed or remove it from txRing
				// env.State = MsgStateFailed
				// c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
				return
			}
			timeout := c.getPacketTimeout(env.Tries)
			c.link.SetPacketTimeout(packet, c.handleTimeout, timeout)
			break
		}
	}
}

// handleDelivered handles packet delivery confirmations
func (c *Channel) handleDelivered(packet any) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for i, env := range c.txRing {
		if env.Packet == packet {
			c.txRing = append(c.txRing[:i], c.txRing[i+1:]...)
			break
		}
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

	for _, entry := range c.messageHandlers {
		if entry.handler != nil {
			msg := &GenericMessage{
				Type: msgType,
				Data: msgData,
				Seq:  sequence,
			}
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

// Close releases channel resources.
func (c *Channel) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Cleanup resources
	return nil
}
