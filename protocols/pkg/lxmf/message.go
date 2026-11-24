// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

// Signer signs packed message bytes (implemented by identity.Identity and similar).
type Signer interface {
	Sign(data []byte) ([]byte, error)
}

// SourceResolver looks up the source identity for inbound signature checks. Nil identity without error leaves signature unverified.
type SourceResolver func(sourceHash []byte) (*identity.Identity, error)

// LXMessage is a decoded or in-flight LXMF message. Use NewMessage or Unpack.
type LXMessage struct {
	DestinationHash []byte
	SourceHash      []byte
	Title           []byte
	Content         []byte
	Fields          map[byte]any
	Timestamp       float64
	Stamp           []byte

	Hash      []byte
	Signature []byte
	Packed    []byte

	State              byte
	Method             byte
	Representation     byte
	Incoming           bool
	SignatureValidated bool
	UnverifiedReason   byte

	// IncludeTicket requests a new outbound ticket attachment when sending.
	IncludeTicket bool
	// OutboundTicket is the optional 16-byte ticket for stamp short-circuit.
	OutboundTicket []byte
	// StampCost is the required delivery stamp cost. Nil means unset,
	// matching upstream lxm.stamp_cost None; the messenger auto-fills
	// it from announced stamp costs before sending.
	StampCost *int
	// StampValue is the PoW score or StampValueTicket after ValidateStamp.
	StampValue int
	// StampValid reflects the last ValidateStamp outcome.
	StampValid bool

	PropagationStamp  []byte
	PropagationPacked []byte
	TransientID       []byte
}

// NewMessage builds a message in StateGenerating with the given hashes, title, content, and optional fields.
func NewMessage(destinationHash, sourceHash, title, content []byte, fields map[byte]any) (*LXMessage, error) {
	if len(destinationHash) != DestinationLength {
		return nil, fmt.Errorf("destination: %w: expected %d, got %d", ErrInvalidHashLength, DestinationLength, len(destinationHash))
	}
	if len(sourceHash) != DestinationLength {
		return nil, fmt.Errorf("source: %w: expected %d, got %d", ErrInvalidHashLength, DestinationLength, len(sourceHash))
	}

	return &LXMessage{
		DestinationHash: append([]byte(nil), destinationHash...),
		SourceHash:      append([]byte(nil), sourceHash...),
		Title:           append([]byte(nil), title...),
		Content:         append([]byte(nil), content...),
		Fields:          copyFields(fields),
		State:           StateGenerating,
		Method:          MethodUnknown,
		Representation:  RepresentationUnknown,
	}, nil
}

func (m *LXMessage) SetTitle(title string) {
	if m == nil {
		return
	}
	m.Title = []byte(title)
}

func (m *LXMessage) SetContent(content string) {
	if m == nil {
		return
	}
	m.Content = []byte(content)
}

func (m *LXMessage) TitleString() string {
	if m == nil {
		return ""
	}
	return string(m.Title)
}

func (m *LXMessage) ContentString() string {
	if m == nil {
		return ""
	}
	return string(m.Content)
}

// FormatHash hex-encodes Hash.
func (m *LXMessage) FormatHash() string {
	if m == nil {
		return "<nil>"
	}
	return hex.EncodeToString(m.Hash)
}

// MessageID is an alias for Hash (stamps and tickets).
func (m *LXMessage) MessageID() []byte {
	if m == nil {
		return nil
	}
	return m.Hash
}

// PackedSize returns len(Packed), or 0 if not packed.
func (m *LXMessage) PackedSize() int {
	if m == nil {
		return 0
	}
	return len(m.Packed)
}

// ContentSize returns packed content length after Pack (upstream len(payload)-timestamp-struct overhead).
func (m *LXMessage) ContentSize() (int, error) {
	if m == nil {
		return 0, errors.New("lxmf: nil message")
	}
	if len(m.Packed) == 0 {
		return 0, errors.New("lxmf: message has not been packed")
	}
	payloadLen := len(m.Packed) - 2*DestinationLength - SignatureLength
	if payloadLen <= TimestampSize+StructOverhead {
		return 0, nil
	}
	return payloadLen - TimestampSize - StructOverhead, nil
}

// GetField returns the raw field value and whether it exists.
func (m *LXMessage) GetField(key byte) (any, bool) {
	if m == nil || m.Fields == nil {
		return nil, false
	}
	val, ok := m.Fields[key]
	return val, ok
}

// GetFieldString returns the field value as a string. Returns empty string if missing or not a string/bytes.
func (m *LXMessage) GetFieldString(key byte) string {
	val, ok := m.GetField(key)
	if !ok {
		return ""
	}
	switch x := val.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}

// GetFieldBytes returns the field value as a byte slice. Returns nil if missing or not a string/bytes.
func (m *LXMessage) GetFieldBytes(key byte) []byte {
	val, ok := m.GetField(key)
	if !ok {
		return nil
	}
	switch x := val.(type) {
	case []byte:
		return append([]byte(nil), x...)
	case string:
		return []byte(x)
	default:
		return nil
	}
}

// GetFieldInt returns the field value as an int64. Returns 0 if missing or not an integer.
func (m *LXMessage) GetFieldInt(key byte) int64 {
	val, ok := m.GetField(key)
	if !ok {
		return 0
	}
	if v, ok := asInt64(val); ok {
		return v
	}
	return 0
}

// GetFieldBool returns the field value as a bool. Returns false if missing or not a bool.
func (m *LXMessage) GetFieldBool(key byte) bool {
	val, ok := m.GetField(key)
	if !ok {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// SetField sets a field value, initializing the Fields map if it was nil.
func (m *LXMessage) SetField(key byte, val any) {
	if m == nil {
		return
	}
	if m.Fields == nil {
		m.Fields = make(map[byte]any)
	}
	m.Fields[key] = val
}

// MaxContentForMethod returns the content cap for method and destination type (ok false if no single-packet limit).
func MaxContentForMethod(method byte, destinationType byte) (int, bool) {
	switch method {
	case MethodOpportunistic:
		switch destinationType {
		case DestinationTypeSingle:
			return EncryptedPacketMaxContent, true
		case DestinationTypePlain:
			return PlainPacketMaxContent, true
		default:
			return EncryptedPacketMaxContent, true
		}
	case MethodDirect, MethodPropagated:
		return LinkPacketMaxContent, true
	case MethodPaper:
		return PaperMDU, true
	default:
		return 0, false
	}
}

// ChooseDeliveryMethod selects method and representation from packed size; MethodUnknown becomes MethodDirect.
func (m *LXMessage) ChooseDeliveryMethod(desiredMethod, destinationType byte) (method, representation byte, err error) {
	if m == nil {
		return 0, 0, errors.New("lxmf: nil message")
	}
	if len(m.Packed) == 0 {
		return 0, 0, errors.New("lxmf: message has not been packed")
	}
	if desiredMethod == MethodUnknown {
		desiredMethod = MethodDirect
	}
	contentSize, err := m.ContentSize()
	if err != nil {
		return 0, 0, err
	}

	switch desiredMethod {
	case MethodOpportunistic:
		limit, ok := MaxContentForMethod(MethodOpportunistic, destinationType)
		if !ok {
			return 0, 0, fmt.Errorf("lxmf: unsupported destination type %d for opportunistic delivery", destinationType)
		}
		if contentSize > limit {
			return MethodDirect, RepresentationPacket, nil
		}
		return MethodOpportunistic, RepresentationPacket, nil

	case MethodDirect, MethodPropagated:
		if contentSize <= LinkPacketMaxContent {
			return desiredMethod, RepresentationPacket, nil
		}
		return desiredMethod, RepresentationResource, nil

	case MethodPaper:
		if len(m.Packed) > PaperMDU {
			return 0, 0, fmt.Errorf("lxmf: packed size %d exceeds paper MDU %d", len(m.Packed), PaperMDU)
		}
		return MethodPaper, RepresentationUnknown, nil

	default:
		return 0, 0, fmt.Errorf("lxmf: unsupported delivery method %d", desiredMethod)
	}
}

// DetermineTransportEncryption returns whether the path is encrypted and the container transport_encryption string.
func DetermineTransportEncryption(method, destinationType byte) (encrypted bool, description string) {
	switch method {
	case MethodOpportunistic, MethodPropagated, MethodPaper:
		switch destinationType {
		case DestinationTypeSingle:
			return true, EncryptionDescriptionEC
		case DestinationTypeGroup:
			return true, EncryptionDescriptionAES
		default:
			return false, EncryptionDescriptionUnencrypted
		}
	case MethodDirect:
		return true, EncryptionDescriptionEC
	default:
		return false, EncryptionDescriptionUnencrypted
	}
}

// ValidateStamp checks PoW stamp or ticket-derived stamp; sets StampValid and StampValue on success.
func (m *LXMessage) ValidateStamp(targetCost int, tickets [][]byte) (bool, error) {
	if m == nil {
		return false, errors.New("lxmf: nil message")
	}
	if len(m.Hash) == 0 {
		return false, errors.New("lxmf: message has no hash; pack or unpack it first")
	}

	if len(tickets) > 0 && len(m.Stamp) > 0 {
		for _, ticket := range tickets {
			if len(ticket) != TicketLength {
				continue
			}
			expected := truncatedHash(ticket, m.Hash)
			if bytes.Equal(expected, m.Stamp) {
				m.StampValue = StampValueTicket
				m.StampValid = true
				return true, nil
			}
		}
	}

	if len(m.Stamp) == 0 {
		m.StampValid = false
		return false, nil
	}

	wb, err := StampWorkblock(m.Hash, WorkblockExpandRounds)
	if err != nil {
		return false, err
	}
	if !StampValid(m.Stamp, targetCost, wb) {
		m.StampValid = false
		return false, nil
	}
	m.StampValue = StampValue(wb, m.Stamp)
	m.StampValid = true
	return true, nil
}

func truncatedHash(ticket, messageID []byte) []byte {
	var buf [TicketLength + sha256.Size]byte
	n := copy(buf[:], ticket)
	n += copy(buf[n:], messageID)
	sum := sha256.Sum256(buf[:n])
	out := make([]byte, DestinationLength)
	copy(out, sum[:DestinationLength])
	return out
}

func (m *LXMessage) String() string {
	if len(m.Hash) == 0 {
		return "<LXMessage>"
	}
	return fmt.Sprintf("<LXMessage %s>", m.FormatHash())
}

func (m *LXMessage) payloadList(includeStamp bool) []any {
	payload := []any{m.Timestamp, m.Title, m.Content, m.Fields}
	if includeStamp && len(m.Stamp) > 0 {
		payload = append(payload, m.Stamp)
	}
	return payload
}

// Pack msgpack-encodes the payload, signs, and fills Packed.
func (m *LXMessage) Pack(signer Signer) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("lxmf: signer required to pack message")
	}
	if len(m.DestinationHash) != DestinationLength {
		return nil, fmt.Errorf("destination: %w", ErrInvalidHashLength)
	}
	if len(m.SourceHash) != DestinationLength {
		return nil, fmt.Errorf("source: %w", ErrInvalidHashLength)
	}

	if m.Timestamp == 0 {
		m.Timestamp = float64(time.Now().UnixNano()) / 1e9
	}

	hashedPayload, err := encodePayload(m.payloadList(false))
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}

	hashedLen := 2*DestinationLength + len(hashedPayload)
	signedPart := make([]byte, hashedLen+sha256.Size)
	copy(signedPart[0:DestinationLength], m.DestinationHash)
	copy(signedPart[DestinationLength:2*DestinationLength], m.SourceHash)
	copy(signedPart[2*DestinationLength:hashedLen], hashedPayload)
	sum := sha256.Sum256(signedPart[:hashedLen])
	m.Hash = append([]byte(nil), sum[:]...)
	copy(signedPart[hashedLen:], sum[:])

	signature, err := signer.Sign(signedPart)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	if len(signature) != SignatureLength {
		return nil, fmt.Errorf("unexpected signature length: %d", len(signature))
	}
	m.Signature = signature
	m.SignatureValidated = true

	packedPayload := hashedPayload
	if len(m.Stamp) > 0 {
		packedPayload, err = encodePayload(m.payloadList(true))
		if err != nil {
			return nil, fmt.Errorf("encode payload with stamp: %w", err)
		}
	}

	packed := make([]byte, 0, 2*DestinationLength+SignatureLength+len(packedPayload))
	packed = append(packed, m.DestinationHash...)
	packed = append(packed, m.SourceHash...)
	packed = append(packed, signature...)
	packed = append(packed, packedPayload...)

	m.Packed = packed
	return packed, nil
}

// EncryptedPayload returns the ciphertext slice after the destination hash (for opportunistic send).
func (m *LXMessage) EncryptedPayload() ([]byte, error) {
	if m == nil {
		return nil, errors.New("lxmf: nil message")
	}
	if m.Packed == nil {
		return nil, errors.New("lxmf: message has not been packed")
	}
	return m.Packed[DestinationLength:], nil
}

// PackPropagated builds propagation_packed for delivery via a propagation node.
// recipient must be an outbound lxmf.delivery destination for the message recipient.
func (m *LXMessage) PackPropagated(recipient *destination.Destination, pnStampCost int) error {
	if m == nil {
		return errors.New("lxmf: nil message")
	}
	if recipient == nil {
		return errors.New("lxmf: nil recipient destination")
	}
	if len(m.Packed) == 0 {
		return errors.New("lxmf: message has not been packed")
	}

	encrypted, err := recipient.Encrypt(m.Packed[DestinationLength:])
	if err != nil {
		return fmt.Errorf("encrypt for propagation: %w", err)
	}

	lxmfData := make([]byte, 0, DestinationLength+len(encrypted))
	lxmfData = append(lxmfData, m.Packed[:DestinationLength]...)
	lxmfData = append(lxmfData, encrypted...)

	transientSum := sha256.Sum256(lxmfData)
	m.TransientID = transientSum[:]

	if pnStampCost > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		stamp, _, err := GenerateStamp(ctx, m.TransientID, pnStampCost, WorkblockExpandRoundsPN)
		if err != nil {
			return fmt.Errorf("propagation stamp: %w", err)
		}
		m.PropagationStamp = stamp
		lxmfData = append(lxmfData, m.PropagationStamp...)
	}

	outer, err := msgpack.Marshal([]any{float64(time.Now().UnixNano()) / 1e9, []any{lxmfData}})
	if err != nil {
		return fmt.Errorf("encode propagation payload: %w", err)
	}
	m.PropagationPacked = outer
	m.Method = MethodPropagated
	m.Representation = RepresentationPacket
	if len(outer) > LinkPacketMaxContent {
		m.Representation = RepresentationResource
	}
	return nil
}

// Unpack parses a full on-wire blob starting with destination hash.
func Unpack(data []byte, resolver SourceResolver) (*LXMessage, error) {
	if len(data) < 2*DestinationLength+SignatureLength {
		return nil, fmt.Errorf("%w: minimum %d bytes", ErrMessageTooShort, 2*DestinationLength+SignatureLength)
	}

	dst := append([]byte(nil), data[:DestinationLength]...)
	return UnpackFromBytes(dst, data[DestinationLength:], resolver)
}

// UnpackFromBytes parses inner LXMF bytes when destinationHash is known out-of-band.
func UnpackFromBytes(destinationHash, inner []byte, resolver SourceResolver) (*LXMessage, error) {
	if len(destinationHash) != DestinationLength {
		return nil, fmt.Errorf("destination: %w", ErrInvalidHashLength)
	}
	if len(inner) < DestinationLength+SignatureLength {
		return nil, fmt.Errorf("%w: minimum %d bytes", ErrMessageTooShort, DestinationLength+SignatureLength)
	}

	source := append([]byte(nil), inner[:DestinationLength]...)
	signature := append([]byte(nil), inner[DestinationLength:DestinationLength+SignatureLength]...)
	packedPayload := inner[DestinationLength+SignatureLength:]

	payload, hashedPayload, err := decodePayloadAndSplit(packedPayload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}

	m := &LXMessage{
		DestinationHash:    append([]byte(nil), destinationHash...),
		SourceHash:         source,
		Signature:          signature,
		Incoming:           true,
		State:              StateGenerating,
		Method:             MethodUnknown,
		Representation:     RepresentationUnknown,
		SignatureValidated: false,
	}

	if err := m.applyPayload(payload); err != nil {
		return nil, err
	}

	hashedLen := 2*DestinationLength + len(hashedPayload)
	signedPart := make([]byte, hashedLen+sha256.Size)
	copy(signedPart[0:DestinationLength], destinationHash)
	copy(signedPart[DestinationLength:2*DestinationLength], source)
	copy(signedPart[2*DestinationLength:hashedLen], hashedPayload)
	sum := sha256.Sum256(signedPart[:hashedLen])
	m.Hash = append([]byte(nil), sum[:]...)
	copy(signedPart[hashedLen:], sum[:])

	packed := make([]byte, 0, 2*DestinationLength+SignatureLength+len(packedPayload))
	packed = append(packed, destinationHash...)
	packed = append(packed, source...)
	packed = append(packed, signature...)
	packed = append(packed, packedPayload...)
	m.Packed = packed

	if resolver != nil {
		sourceID, resolveErr := resolver(source)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve source: %w", resolveErr)
		}
		if sourceID == nil {
			m.UnverifiedReason = UnverifiedSourceUnknown
			return m, nil
		}
		if !sourceID.Verify(signedPart, signature) {
			m.UnverifiedReason = UnverifiedSignatureInvalid
			return m, ErrSignatureInvalid
		}
		m.SignatureValidated = true
	} else {
		m.UnverifiedReason = UnverifiedSourceUnknown
	}

	return m, nil
}

// RecallSource wraps identity.Recall (nil identity if missing).
func RecallSource(sourceHash []byte) (*identity.Identity, error) {
	id, err := identity.Recall(sourceHash)
	if err != nil {
		return nil, nil
	}
	return id, nil
}

func (m *LXMessage) applyPayload(payload []any) error {
	if len(payload) < 4 {
		return fmt.Errorf("%w: expected 4 elements, got %d", ErrInvalidPayload, len(payload))
	}

	switch v := payload[0].(type) {
	case float64:
		m.Timestamp = v
	case float32:
		m.Timestamp = float64(v)
	case int8:
		m.Timestamp = float64(v)
	case int16:
		m.Timestamp = float64(v)
	case int32:
		m.Timestamp = float64(v)
	case int64:
		m.Timestamp = float64(v)
	case uint8:
		m.Timestamp = float64(v)
	case uint16:
		m.Timestamp = float64(v)
	case uint32:
		m.Timestamp = float64(v)
	case uint64:
		m.Timestamp = float64(v)
	default:
		return fmt.Errorf("%w: unexpected timestamp type %T", ErrInvalidPayload, v)
	}

	title, err := asBytes(payload[1])
	if err != nil {
		return fmt.Errorf("title: %w", err)
	}
	m.Title = title

	content, err := asBytes(payload[2])
	if err != nil {
		return fmt.Errorf("content: %w", err)
	}
	m.Content = content

	fields, err := asFields(payload[3])
	if err != nil {
		return fmt.Errorf("fields: %w", err)
	}
	m.Fields = fields

	if len(payload) > 4 {
		stamp, err := asBytes(payload[4])
		if err != nil {
			return fmt.Errorf("stamp: %w", err)
		}
		m.Stamp = stamp
	}

	return nil
}

var payloadBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

var payloadReaderPool = sync.Pool{
	New: func() any { return bytes.NewReader(nil) },
}

const maxPooledPayloadBuf = 1 << 20

func encodePayload(payload []any) ([]byte, error) {
	buf := payloadBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := msgpack.GetEncoder()
	enc.Reset(buf)
	enc.UseCompactInts(true)
	enc.UseCompactFloats(false)
	enc.UseInternedStrings(false)
	err := enc.Encode(payload)
	msgpack.PutEncoder(enc)
	if err != nil {
		putPayloadBuf(buf)
		return nil, err
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	putPayloadBuf(buf)
	return out, nil
}

func putPayloadBuf(buf *bytes.Buffer) {
	if buf.Cap() <= maxPooledPayloadBuf {
		payloadBufPool.Put(buf)
	}
}

// decodePayloadAndSplit decodes payload and preserves raw bytes for hashing (multi-field map order stability).
func decodePayloadAndSplit(data []byte) ([]any, []byte, error) {
	r := payloadReaderPool.Get().(*bytes.Reader)
	r.Reset(data)
	dec := msgpack.GetDecoder()
	dec.Reset(r)
	dec.UseLooseInterfaceDecoding(true)
	mapCtx := &msgpackMapCtx{maxDepth: msgpackMapMaxDepth, maxPairs: msgpackMapMaxPairs}
	dec.SetMapDecoder(mapCtx.decodeMap)
	defer func() {
		msgpack.PutDecoder(dec)
		r.Reset(nil)
		payloadReaderPool.Put(r)
	}()

	totalLen := int64(len(data))
	arrLen, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, nil, err
	}
	if arrLen < 4 {
		return nil, nil, fmt.Errorf("expected at least 4 elements, got %d", arrLen)
	}
	if arrLen > 15 {
		return nil, nil, fmt.Errorf("payload array too long: %d", arrLen)
	}

	posAfterHeader := totalLen - int64(r.Len())

	out := make([]any, 0, arrLen)
	for range 4 {
		v, decErr := dec.DecodeInterface()
		if decErr != nil {
			return nil, nil, decErr
		}
		out = append(out, v)
	}
	posAfterFourth := totalLen - int64(r.Len())

	var hashed []byte
	if arrLen == 4 {
		hashed = append([]byte(nil), data...)
	} else {
		body := data[posAfterHeader:posAfterFourth]
		hashed = make([]byte, 0, 1+len(body))
		hashed = append(hashed, 0x94)
		hashed = append(hashed, body...)
	}

	for i := 4; i < arrLen; i++ {
		v, decErr := dec.DecodeInterface()
		if decErr != nil {
			return nil, nil, decErr
		}
		out = append(out, v)
	}
	if r.Len() != 0 {
		return nil, nil, fmt.Errorf("%w: trailing payload bytes", ErrInvalidPayload)
	}
	if arrLen != 4 && arrLen != 5 {
		return nil, nil, fmt.Errorf("%w: unexpected payload array length %d", ErrInvalidPayload, arrLen)
	}
	return out, hashed, nil
}

// msgpack map decode limits (DoS bounds).
const (
	msgpackMapMaxDepth = 16
	msgpackMapMaxPairs = 1024
)

type msgpackMapCtx struct {
	depth    int
	maxDepth int
	maxPairs int
}

func (c *msgpackMapCtx) decodeMap(d *msgpack.Decoder) (any, error) {
	if c.depth >= c.maxDepth {
		return nil, fmt.Errorf("%w: msgpack map nesting exceeds %d", ErrInvalidPayload, c.maxDepth)
	}
	c.depth++
	defer func() { c.depth-- }()

	n, err := d.DecodeMapLen()
	if err != nil {
		return nil, err
	}
	if n == -1 {
		return nil, nil
	}
	if n > c.maxPairs {
		return nil, fmt.Errorf("%w: msgpack map too large (%d entries)", ErrInvalidPayload, n)
	}
	out := make(map[any]any, n)
	for range n {
		k, err := d.DecodeInterface()
		if err != nil {
			return nil, err
		}
		if k != nil && !reflect.TypeOf(k).Comparable() {
			return nil, fmt.Errorf("%w: msgpack map key type %T cannot be used as a map key", ErrInvalidPayload, k)
		}
		v, err := d.DecodeInterface()
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func copyFields(in map[byte]any) map[byte]any {
	if in == nil {
		return make(map[byte]any)
	}
	out := make(map[byte]any, len(in))
	maps.Copy(out, in)
	return out
}

func asBytes(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), x...), nil
	case string:
		return []byte(x), nil
	default:
		return nil, fmt.Errorf("%w: expected bytes, got %T", ErrInvalidPayload, v)
	}
}

func asFields(v any) (map[byte]any, error) {
	if v == nil {
		return make(map[byte]any), nil
	}
	m, ok := v.(map[any]any)
	if !ok {
		if mm, ok2 := v.(map[string]any); ok2 {
			out := make(map[byte]any, len(mm))
			for k, val := range mm {
				key, err := fieldKeyFromString(k)
				if err != nil {
					return nil, err
				}
				normalized, err := normalizeFieldValue(val)
				if err != nil {
					return nil, err
				}
				out[key] = normalized
			}
			return out, nil
		}
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidPayload, v)
	}
	out := make(map[byte]any, len(m))
	for k, val := range m {
		key, err := asByteKey(k)
		if err != nil {
			return nil, err
		}
		normalized, err := normalizeFieldValue(val)
		if err != nil {
			return nil, err
		}
		out[key] = normalized
	}
	return out, nil
}

func normalizeFieldValue(v any) (any, error) {
	switch x := v.(type) {
	case map[any]any:
		return asNestedValue(x)
	case map[string]any:
		return asNestedValue(x)
	case map[byte]any:
		out := make(map[byte]any, len(x))
		for k, val := range x {
			normalized, err := normalizeFieldValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, elem := range x {
			normalized, err := normalizeFieldValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	default:
		return v, nil
	}
}

// asNestedValue normalizes a map found inside a field value. LXMF field
// values are arbitrary msgpack data: a nested map may be another LXMF field
// map (byte-encodable keys) or an application dict with named keys (command
// entries, reaction dicts). Try the strict field-map decode first; when keys
// are not byte-encodable, preserve the map as map[string]any so named-key
// payloads from Python LXMFy and other clients survive the round trip.
func asNestedValue(v any) (any, error) {
	if strict, err := asFields(v); err == nil {
		return strict, nil
	}
	return tolerantNestedMap(v)
}

func tolerantNestedMap(v any) (any, error) {
	out := make(map[string]any)
	switch m := v.(type) {
	case map[string]any:
		for k, val := range m {
			normalized, err := normalizeFieldValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = normalized
		}
	case map[any]any:
		for k, val := range m {
			normalized, err := normalizeFieldValue(val)
			if err != nil {
				return nil, err
			}
			out[nestedKeyString(k)] = normalized
		}
	default:
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidPayload, v)
	}
	return out, nil
}

func nestedKeyString(k any) string {
	switch x := k.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case byte:
		return nestedByteKey(x)
	case int, int8, int16, int32, int64, uint, uint16, uint32, uint64:
		if n, ok := nestedIntKey(x); ok {
			return fmt.Sprintf("0x%02x", n)
		}
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func nestedByteKey(b byte) string {
	if b >= 0x20 && b < 0x7F {
		return string(b)
	}
	return fmt.Sprintf("0x%02x", b)
}

func nestedIntKey(v any) (int, bool) {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int8:
		n = int64(x)
	case int16:
		n = int64(x)
	case int32:
		n = int64(x)
	case int64:
		n = x
	case uint:
		if x > 0xFF {
			return 0, false
		}
		n = int64(x)
	case uint8:
		n = int64(x)
	case uint16:
		n = int64(x)
	case uint32:
		n = int64(x)
	case uint64:
		if x > 0xFF {
			return 0, false
		}
		n = int64(x)
	default:
		return 0, false
	}
	if n < 0 || n > 0xFF {
		return 0, false
	}
	return int(n), true
}

func asByteKey(v any) (byte, error) {
	switch x := v.(type) {
	case int8:
		if x < 0 {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case int16:
		if x < 0 || x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case int32:
		if x < 0 || x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case int64:
		if x < 0 || x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case uint8:
		return x, nil
	case uint16:
		if x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case uint32:
		if x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case uint64:
		if x > 0xFF {
			return 0, fmt.Errorf("%w: field key out of range: %d", ErrInvalidPayload, x)
		}
		return byte(x), nil
	case string:
		return fieldKeyFromString(x)
	default:
		return 0, fmt.Errorf("%w: field key has unexpected type %T", ErrInvalidPayload, v)
	}
}

func fieldKeyFromString(s string) (byte, error) {
	if len(s) == 1 {
		return s[0], nil
	}
	if len(s) >= 3 && (s[0:2] == "0x" || s[0:2] == "0X") {
		n, err := parseHexByte(s[2:])
		if err != nil {
			return 0, fmt.Errorf("%w: field key %q: %v", ErrInvalidPayload, s, err)
		}
		return n, nil
	}
	return 0, fmt.Errorf("%w: field key must encode as single byte, got %q", ErrInvalidPayload, s)
}

func parseHexByte(s string) (byte, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty hex")
	}
	var n byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, fmt.Errorf("invalid hex digit")
		}
		n = (n << 4) | d
	}
	return n, nil
}

// StateString returns a human-readable string for the message state.
func StateString(state byte) string {
	switch state {
	case StateGenerating:
		return "Generating"
	case StateOutbound:
		return "Outbound"
	case StateSending:
		return "Sending"
	case StateSent:
		return "Sent"
	case StateDelivered:
		return "Delivered"
	case StateRejected:
		return "Rejected"
	case StateCancelled:
		return "Cancelled"
	case StateFailed:
		return "Failed"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", state)
	}
}

// MethodString returns a human-readable string for the delivery method.
func MethodString(method byte) string {
	switch method {
	case MethodUnknown:
		return "Unknown"
	case MethodOpportunistic:
		return "Opportunistic"
	case MethodDirect:
		return "Direct"
	case MethodPropagated:
		return "Propagated"
	case MethodPaper:
		return "Paper"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", method)
	}
}

// RepresentationString returns a human-readable string for the representation.
func RepresentationString(rep byte) string {
	switch rep {
	case RepresentationUnknown:
		return "Unknown"
	case RepresentationPacket:
		return "Packet"
	case RepresentationResource:
		return "Resource"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", rep)
	}
}

// FieldName returns a human-readable name for a payload field key.
func FieldName(key byte) string {
	switch key {
	case FieldEmbeddedLXMs:
		return "EmbeddedLXMs"
	case FieldTelemetry:
		return "Telemetry"
	case FieldTelemetryStream:
		return "TelemetryStream"
	case FieldIconAppearance:
		return "IconAppearance"
	case FieldFileAttachments:
		return "FileAttachments"
	case FieldImage:
		return "Image"
	case FieldAudio:
		return "Audio"
	case FieldThread:
		return "Thread"
	case FieldCommands:
		return "Commands"
	case FieldResults:
		return "Results"
	case FieldGroup:
		return "Group"
	case FieldTicket:
		return "Ticket"
	case FieldEvent:
		return "Event"
	case FieldRNRRefs:
		return "RNRRefs"
	case FieldRenderer:
		return "Renderer"
	case FieldReplyTo:
		return "ReplyTo"
	case FieldReplyQuote:
		return "ReplyQuote"
	case FieldReaction:
		return "Reaction"
	case FieldComment:
		return "Comment"
	case FieldContinuation:
		return "Continuation"
	case FieldCustomType:
		return "CustomType"
	case FieldCustomData:
		return "CustomData"
	case FieldCustomMeta:
		return "CustomMeta"
	case FieldNonSpecific:
		return "NonSpecific"
	case FieldDebug:
		return "Debug"
	default:
		return fmt.Sprintf("Custom(0x%02X)", key)
	}
}

// Diagnostics returns a multi-line detailed diagnostic report of the message.
func (m *LXMessage) Diagnostics() string {
	if m == nil {
		return "LXMessage: <nil>"
	}

	var sb strings.Builder
	sb.WriteString("LXMF Message Diagnostics:\n")
	fmt.Fprintf(&sb, "  Message ID (Hash):  %s\n", m.FormatHash())
	fmt.Fprintf(&sb, "  State:              %s\n", StateString(m.State))
	fmt.Fprintf(&sb, "  Delivery Method:    %s\n", MethodString(m.Method))
	fmt.Fprintf(&sb, "  Representation:     %s\n", RepresentationString(m.Representation))
	fmt.Fprintf(&sb, "  Destination Hash:   %s\n", hex.EncodeToString(m.DestinationHash))
	fmt.Fprintf(&sb, "  Source Hash:        %s\n", hex.EncodeToString(m.SourceHash))

	if m.Timestamp > 0 {
		sec := int64(m.Timestamp)
		nsec := int64((m.Timestamp - float64(sec)) * 1e9)
		t := time.Unix(sec, nsec).UTC()
		fmt.Fprintf(&sb, "  Timestamp:          %.6f (%s)\n", m.Timestamp, t.Format(time.RFC3339Nano))
	} else {
		sb.WriteString("  Timestamp:          0 (Not set)\n")
	}

	fmt.Fprintf(&sb, "  Title:              %q\n", m.TitleString())
	fmt.Fprintf(&sb, "  Content Length:     %d bytes\n", len(m.Content))

	if len(m.Fields) > 0 {
		sb.WriteString("  Fields:\n")
		// Sort keys for deterministic output
		keys := make([]byte, 0, len(m.Fields))
		for k := range m.Fields {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		for _, k := range keys {
			val := m.Fields[k]
			fmt.Fprintf(&sb, "    - %-16s (0x%02X): %+v\n", FieldName(k), k, val)
		}
	} else {
		sb.WriteString("  Fields:             None\n")
	}

	if len(m.Stamp) > 0 {
		fmt.Fprintf(&sb, "  Stamp:              %s (Value: %d, Valid: %t)\n", hex.EncodeToString(m.Stamp), m.StampValue, m.StampValid)
	} else {
		sb.WriteString("  Stamp:              None\n")
	}

	if m.SignatureValidated {
		sb.WriteString("  Signature:          Validated\n")
	} else {
		reason := "Unknown"
		switch m.UnverifiedReason {
		case UnverifiedSourceUnknown:
			reason = "Source identity unknown"
		case UnverifiedSignatureInvalid:
			reason = "Signature invalid"
		}
		fmt.Fprintf(&sb, "  Signature:          Unverified (%s)\n", reason)
	}

	if len(m.Packed) > 0 {
		fmt.Fprintf(&sb, "  Packed Size:        %d bytes\n", len(m.Packed))
	} else {
		sb.WriteString("  Packed Size:        Not packed\n")
	}

	return sb.String()
}
