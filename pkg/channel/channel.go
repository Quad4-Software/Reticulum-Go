package channel

import (
	"errors"
	"math"
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/debug"
	"git.quad4.io/Networks/Reticulum-Go/pkg/transport"
)

const (
	// Window sizes and thresholds
	WindowInitial     = 2
	WindowMin         = 2
	WindowMinSlow     = 2
	WindowMinMedium   = 5
	WindowMinFast     = 16
	WindowMaxSlow     = 5
	WindowMaxMedium   = 12
	WindowMaxFast     = 48
	WindowMax         = WindowMaxFast
	WindowFlexibility = 4

	// RTT thresholds
	RTTFast   = 0.18
	RTTMedium = 0.75
	RTTSlow   = 1.45

	// Sequence numbers
	SeqMax     uint16 = 0xFFFF
	SeqModulus uint16 = SeqMax

	FastRateThreshold = 10

	// Timeout calculation constants
	RTTMinThreshold       = 0.025
	TimeoutBaseMultiplier = 1.5
	TimeoutRingMultiplier = 2.5
	TimeoutRingOffset     = 2

	// Packet header constants
	ChannelHeaderSize = 6
	ChannelHeaderBits = 8

	// Default retry count
	DefaultMaxTries = 3
)

// MessageState represents the state of a message
type MessageState int

const (
	MsgStateNew MessageState = iota
	MsgStateSent
	MsgStateDelivered
	MsgStateFailed
)

// MessageBase defines the interface for messages that can be sent over a channel
type MessageBase interface {
	Pack() ([]byte, error)
	Unpack([]byte) error
	GetType() uint16
}

// Channel manages reliable message delivery over a transport link
type Channel struct {
	link            transport.LinkInterface
	mutex           sync.RWMutex
	txRing          []*Envelope
	rxRing          []*Envelope
	window          int
	windowMax       int
	windowMin       int
	windowFlex      int
	nextSequence    uint16
	nextRxSequence  uint16
	maxTries        int
	fastRateRounds  int
	medRateRounds   int
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
	Packet    interface{}
	Tries     int
	Timestamp time.Time
}

// NewChannel creates a new Channel instance
func NewChannel(link transport.LinkInterface) *Channel {
	return &Channel{
		link:            link,
		messageHandlers: make([]messageHandlerEntry, 0),
		mutex:           sync.RWMutex{},
		windowMax:       WindowMaxSlow,
		windowMin:       WindowMinSlow,
		window:          WindowInitial,
		maxTries:        DefaultMaxTries,
	}
}

// Send transmits a message over the channel
func (c *Channel) Send(msg MessageBase) error {
	if c.link.GetStatus() != transport.STATUS_ACTIVE {
		return errors.New("link not ready")
	}

	env := &Envelope{
		Sequence:  c.nextSequence,
		Message:   msg,
		Timestamp: time.Now(),
	}

	c.mutex.Lock()
	c.nextSequence = (c.nextSequence + common.ONE) % SeqModulus
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
func (c *Channel) handleTimeout(packet interface{}) {
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
				debug.Log(debug.DEBUG_INFO, "Failed to resend packet", "error", err)
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
func (c *Channel) handleDelivered(packet interface{}) {
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

	timeout := math.Pow(TimeoutBaseMultiplier, float64(tries-common.ONE)) * rtt * TimeoutRingMultiplier * float64(len(c.txRing)+TimeoutRingOffset)
	return time.Duration(timeout * float64(time.Second))
}

func (c *Channel) AddMessageHandler(handler func(MessageBase) bool) int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	id := c.nextHandlerID
	c.nextHandlerID++
	c.messageHandlers = append(c.messageHandlers, messageHandlerEntry{id: id, handler: handler})
	return id
}

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

func (c *Channel) updateRateThresholds() {
	rtt := c.link.RTT()

	if rtt > RTTFast {
		c.fastRateRounds = common.ZERO

		if rtt > RTTMedium {
			c.medRateRounds = common.ZERO
		} else {
			c.medRateRounds++
			if c.windowMax < WindowMaxMedium && c.medRateRounds == FastRateThreshold {
				c.windowMax = WindowMaxMedium
				c.windowMin = WindowMinMedium
			}
		}
	} else {
		c.fastRateRounds++
		if c.windowMax < WindowMaxFast && c.fastRateRounds == FastRateThreshold {
			c.windowMax = WindowMaxFast
			c.windowMin = WindowMinFast
		}
	}
}

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

type GenericMessage struct {
	Type uint16
	Data []byte
	Seq  uint16
}

func (g *GenericMessage) Pack() ([]byte, error) {
	return g.Data, nil
}

func (g *GenericMessage) Unpack(data []byte) error {
	g.Data = data
	return nil
}

func (g *GenericMessage) GetType() uint16 {
	return g.Type
}

func (c *Channel) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// Cleanup resources
	return nil
}
