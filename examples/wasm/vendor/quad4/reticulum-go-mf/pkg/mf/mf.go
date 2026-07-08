package mf

import (
	"encoding/hex"
	"fmt"

	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/packet"
	"quad4/reticulum-go/pkg/transport"
)

// Message is a compact MF wire format: sender hash plus UTF-8 text.
// An optional group hash routes fan-out group traffic on 1:1 MF destinations.
type Message struct {
	SenderHash []byte
	GroupHash  []byte
	Text       string
}

// NewMessage validates and returns a Message.
func NewMessage(senderHash []byte, text string) (*Message, error) {
	return NewMessageWithGroup(senderHash, nil, text)
}

// NewMessageWithGroup validates and returns a Message optionally tagged with a group hash.
func NewMessageWithGroup(senderHash, groupHash []byte, text string) (*Message, error) {
	m := &Message{
		SenderHash: senderHash,
		GroupHash:  groupHash,
		Text:       text,
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// NewMessageFromHex parses the sender hash from hex then calls NewMessage.
func NewMessageFromHex(senderHashHex string, text string) (*Message, error) {
	hash, err := hex.DecodeString(senderHashHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hash: %w", err)
	}
	return NewMessage(hash, text)
}

// Validate checks hash length and text size.
func (m *Message) Validate() error {
	if len(m.SenderHash) != SenderHashLength {
		return fmt.Errorf(errFmtExpected, ErrInvalidHashLength, SenderHashLength, len(m.SenderHash))
	}
	if len(m.GroupHash) > 0 && len(m.GroupHash) != SenderHashLength {
		return fmt.Errorf(errFmtExpected, ErrInvalidHashLength, SenderHashLength, len(m.GroupHash))
	}
	if len(m.Text) > MaxMessageSize {
		return fmt.Errorf("%w: max %d, got %d", ErrMessageTooLong, MaxMessageSize, len(m.Text))
	}
	return nil
}

// Len returns packed size.
func (m *Message) Len() int {
	if len(m.GroupHash) > 0 {
		return SenderHashLength + 1 + SenderHashLength + len(m.Text)
	}
	return SenderHashLength + len(m.Text)
}

// FormatSenderHash hex-encodes the sender hash.
func (m *Message) FormatSenderHash() string {
	return hex.EncodeToString(m.SenderHash)
}

func (m *Message) String() string {
	return fmt.Sprintf("Message{Sender: %s, Text: %q}", m.FormatSenderHash(), m.Text)
}

// Equal compares sender hash and text.
func (m *Message) Equal(other *Message) bool {
	if other == nil {
		return false
	}
	if m.Text != other.Text {
		return false
	}
	if len(m.SenderHash) != len(other.SenderHash) {
		return false
	}
	for i := range m.SenderHash {
		if m.SenderHash[i] != other.SenderHash[i] {
			return false
		}
	}
	return true
}

// Pack returns [16-byte sender][UTF-8 text] or [16-byte sender][0x01][16-byte group][UTF-8 text].
func (m *Message) Pack() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if len(m.GroupHash) > 0 {
		payload := make([]byte, m.Len())
		copy(payload[:SenderHashLength], m.SenderHash)
		payload[SenderHashLength] = groupFlagPresent
		copy(payload[SenderHashLength+1:SenderHashLength+1+SenderHashLength], m.GroupHash)
		copy(payload[SenderHashLength+1+SenderHashLength:], []byte(m.Text))
		return payload, nil
	}
	payload := make([]byte, m.Len())
	copy(payload[:SenderHashLength], m.SenderHash)
	copy(payload[SenderHashLength:], []byte(m.Text))
	return payload, nil
}

// Unpack parses Pack output.
func Unpack(data []byte) (*Message, error) {
	if len(data) < SenderHashLength {
		return nil, fmt.Errorf("%w: minimum %d bytes for sender hash", ErrMessageTooShort, SenderHashLength)
	}

	senderHash := make([]byte, SenderHashLength)
	copy(senderHash, data[:SenderHashLength])
	rest := data[SenderHashLength:]
	if len(rest) > 0 && rest[0] == groupFlagPresent {
		if len(rest) < 1+SenderHashLength {
			return nil, fmt.Errorf("%w: truncated group payload", ErrMessageTooShort)
		}
		groupHash := make([]byte, SenderHashLength)
		copy(groupHash, rest[1:1+SenderHashLength])
		text := string(rest[1+SenderHashLength:])
		return NewMessageWithGroup(senderHash, groupHash, text)
	}
	return NewMessage(senderHash, string(rest))
}

// Peer is a discovered peer hash plus app data string.
type Peer struct {
	Hash    []byte
	AppData string
}

// Validate checks hash length.
func (p *Peer) Validate() error {
	if len(p.Hash) != SenderHashLength {
		return fmt.Errorf(errFmtExpected, ErrInvalidHashLength, SenderHashLength, len(p.Hash))
	}
	return nil
}

// FormatHash hex-encodes the peer hash.
func (p *Peer) FormatHash() string {
	return hex.EncodeToString(p.Hash)
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer{Hash: %s, AppData: %q}", p.FormatHash(), p.AppData)
}

// Messenger sends MF over a Reticulum transport and inbound destination.
type Messenger struct {
	transport *transport.Transport
	dest      *destination.Destination
}

// NewMessenger wraps a transport and destination.
func NewMessenger(t *transport.Transport, d *destination.Destination) *Messenger {
	return &Messenger{
		transport: t,
		dest:      d,
	}
}

// GetDestinationHash returns the local destination hash bytes.
func (m *Messenger) GetDestinationHash() []byte {
	return m.dest.GetHash()
}

// GetDestination returns the local RNS destination.
func (m *Messenger) GetDestination() *destination.Destination {
	return m.dest
}

// SendMessage encrypts and sends one MF packet to destHash.
func (m *Messenger) SendMessage(destHash []byte, text string) error {
	if len(destHash) != SenderHashLength {
		return fmt.Errorf("invalid destination hash: %w", ErrInvalidHashLength)
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		return fmt.Errorf("identity not found: %w", err)
	}

	targetDest, err := destination.FromHash(destHash, remoteIdentity, destination.Single, m.transport)
	if err != nil {
		return fmt.Errorf("failed to create target destination: %w", err)
	}

	msg, err := NewMessage(m.GetDestinationHash(), text)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	return m.sendPacked(destHash, targetDest, msg)
}

// SendGroupMessage sends one MF packet to destHash tagged with groupHash.
func (m *Messenger) SendGroupMessage(destHash, groupHash []byte, text string) error {
	if len(destHash) != SenderHashLength {
		return fmt.Errorf("invalid destination hash: %w", ErrInvalidHashLength)
	}
	if len(groupHash) != SenderHashLength {
		return fmt.Errorf("invalid group hash: %w", ErrInvalidHashLength)
	}

	remoteIdentity, err := identity.Recall(destHash)
	if err != nil {
		return fmt.Errorf("identity not found: %w", err)
	}

	targetDest, err := destination.FromHash(destHash, remoteIdentity, destination.Single, m.transport)
	if err != nil {
		return fmt.Errorf("failed to create target destination: %w", err)
	}

	msg, err := NewMessageWithGroup(m.GetDestinationHash(), groupHash, text)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	return m.sendPacked(destHash, targetDest, msg)
}

func (m *Messenger) sendPacked(destHash []byte, targetDest *destination.Destination, msg *Message) error {
	payload, err := msg.Pack()
	if err != nil {
		return fmt.Errorf("failed to pack message: %w", err)
	}

	encrypted, err := targetDest.Encrypt(payload)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	pkt := packet.NewPacket(
		packet.DestinationSingle,
		encrypted,
		packet.PacketTypeData,
		packet.ContextNone,
		packet.PropagationBroadcast,
		packet.HeaderType1,
		nil,
		true,
		packet.FlagUnset,
	)
	pkt.DestinationHash = destHash

	if err := pkt.Pack(); err != nil {
		return fmt.Errorf("packet packing failed: %w", err)
	}

	if err := m.transport.SendPacket(pkt); err != nil {
		return fmt.Errorf("packet sending failed: %w", err)
	}

	return nil
}

// SenderHashFromHex decodes a 32-byte hex sender hash.
func SenderHashFromHex(s string) ([]byte, error) {
	hash, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(hash) != SenderHashLength {
		return nil, fmt.Errorf(errFmtExpected, ErrInvalidHashLength, SenderHashLength, len(hash))
	}
	return hash, nil
}

// ValidateSenderHash checks length == SenderHashLength.
func ValidateSenderHash(hash []byte) error {
	if len(hash) != SenderHashLength {
		return fmt.Errorf(errFmtExpected, ErrInvalidHashLength, SenderHashLength, len(hash))
	}
	return nil
}
