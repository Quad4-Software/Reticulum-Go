// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io

package discovery

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"git.quad4.io/Networks/Reticulum-Go/pkg/cryptography"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

// AppName is the destination app_name used by Discovery (see const value).
const AppName = "rnstransport"

// Aspects is the destination aspect chain used by Discovery
// (discovery.interface).
var Aspects = []string{"discovery", "interface"}

// Field tags from Discovery. Each is a single-byte msgpack map key.
const (
	FieldName            byte = 0xFF
	FieldTransportID     byte = 0xFE
	FieldInterfaceType   byte = 0x00
	FieldTransport       byte = 0x01
	FieldReachableOn     byte = 0x02
	FieldLatitude        byte = 0x03
	FieldLongitude       byte = 0x04
	FieldHeight          byte = 0x05
	FieldPort            byte = 0x06
	FieldIFACNetname     byte = 0x07
	FieldIFACNetkey      byte = 0x08
	FieldFrequency       byte = 0x09
	FieldBandwidth       byte = 0x0A
	FieldSpreadingFactor byte = 0x0B
	FieldCodingRate      byte = 0x0C
	FieldModulation      byte = 0x0D
	FieldChannel         byte = 0x0E
)

// Flag bits from InterfaceAnnounceHandler.
const (
	FlagSigned    byte = 0b00000001
	FlagEncrypted byte = 0b00000010
)

// DefaultStampValue matches InterfaceAnnouncer.DEFAULT_STAMP_VALUE.
const DefaultStampValue = 14

// WorkblockExpandRounds matches InterfaceAnnouncer.WORKBLOCK_EXPAND_ROUNDS
// and is the value used when stamping discovery announcements (much smaller
// than the LXMF defaults).
const WorkblockExpandRounds = 20

// StampSize matches LXMF.LXStamper.STAMP_SIZE (identity HASHLENGTH / 8 = 32).
const StampSize = 32

// Info is the high-level Go representation of a discovery info payload. Only
// fields that were present in the msgpack map are populated; check the
// corresponding Has* boolean before reading optional numeric fields.
type Info struct {
	Type        string
	Transport   bool
	TransportID []byte
	Name        string
	Latitude    float64
	Longitude   float64
	Height      float64
	HasGeo      bool

	ReachableOn string
	Port        int64
	HasPort     bool

	IFACNetname string
	IFACNetkey  string

	Frequency       int64
	Bandwidth       int64
	SpreadingFactor int64
	CodingRate      int64
	Channel         int64
	Modulation      string
}

// EncodeInfo serialises an Info into the msgpack representation that
// Discovery.get_interface_announce_data emits for the info dictionary
// (i.e. the bytes passed to msgpack.packb). Numeric fields are omitted when
// zero unless the matching Has flag is set, because the reference stack pre-populates the
// map with explicit interface defaults; callers that want zero-valued fields
// should pass in the appropriate flag.
func EncodeInfo(in Info) ([]byte, error) {
	if in.Type == "" {
		return nil, errors.New("discovery: Info.Type required")
	}
	if len(in.TransportID) == 0 {
		return nil, errors.New("discovery: Info.TransportID required")
	}

	pairs := make([][2]any, 0, 16)
	pairs = append(pairs,
		[2]any{FieldInterfaceType, in.Type},
		[2]any{FieldTransport, in.Transport},
		[2]any{FieldTransportID, in.TransportID},
		[2]any{FieldName, in.Name},
		[2]any{FieldLatitude, in.Latitude},
		[2]any{FieldLongitude, in.Longitude},
		[2]any{FieldHeight, in.Height},
	)
	if in.ReachableOn != "" {
		pairs = append(pairs, [2]any{FieldReachableOn, in.ReachableOn})
	}
	if in.HasPort {
		pairs = append(pairs, [2]any{FieldPort, in.Port})
	}
	if in.IFACNetname != "" {
		pairs = append(pairs, [2]any{FieldIFACNetname, in.IFACNetname})
	}
	if in.IFACNetkey != "" {
		pairs = append(pairs, [2]any{FieldIFACNetkey, in.IFACNetkey})
	}
	if in.Frequency != 0 {
		pairs = append(pairs, [2]any{FieldFrequency, in.Frequency})
	}
	if in.Bandwidth != 0 {
		pairs = append(pairs, [2]any{FieldBandwidth, in.Bandwidth})
	}
	if in.SpreadingFactor != 0 {
		pairs = append(pairs, [2]any{FieldSpreadingFactor, in.SpreadingFactor})
	}
	if in.CodingRate != 0 {
		pairs = append(pairs, [2]any{FieldCodingRate, in.CodingRate})
	}
	if in.Channel != 0 {
		pairs = append(pairs, [2]any{FieldChannel, in.Channel})
	}
	if in.Modulation != "" {
		pairs = append(pairs, [2]any{FieldModulation, in.Modulation})
	}

	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.EncodeMapLen(len(pairs)); err != nil {
		return nil, err
	}
	for _, kv := range pairs {
		if err := enc.Encode(kv[0]); err != nil {
			return nil, err
		}
		if err := enc.Encode(kv[1]); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// MaxInfoSize bounds the msgpack info payload accepted by DecodeInfo.
// Real-world Discovery info dictionaries are well below this; the cap
// defeats adversarial inputs that declare collection lengths far larger
// than the buffer they actually carry.
const MaxInfoSize = 64 * 1024

// MaxInfoFields bounds the number of fields accepted in a single info
// dictionary. Discovery uses ~16 fields; anything wildly larger is
// rejected to prevent unbounded allocation.
const MaxInfoFields = 256

// MaxAppDataSize bounds the discovery announce app_data buffer accepted
// by DecodeAppData / ValidateAndDecode.
const MaxAppDataSize = 64 * 1024

// DecodeInfo reverses EncodeInfo, parsing the msgpack info map produced by
// Discovery.get_interface_announce_data.
func DecodeInfo(raw []byte) (Info, error) {
	if len(raw) > MaxInfoSize {
		return Info{}, fmt.Errorf("discovery: info payload too large (%d > %d)", len(raw), MaxInfoSize)
	}
	dec := msgpack.NewDecoder(bytes.NewReader(raw))
	dec.UseLooseInterfaceDecoding(true)
	n, err := dec.DecodeMapLen()
	if err != nil {
		return Info{}, fmt.Errorf("discovery: decode map len: %w", err)
	}
	if n < 0 {
		return Info{}, nil
	}
	if n > MaxInfoFields || n > len(raw) {
		return Info{}, fmt.Errorf("discovery: map length %d exceeds bounds (max=%d, payload=%d)", n, MaxInfoFields, len(raw))
	}
	out := Info{}
	for i := 0; i < n; i++ {
		var keyByte byte
		if err := dec.Decode(&keyByte); err != nil {
			return Info{}, fmt.Errorf("discovery: decode key %d: %w", i, err)
		}
		raw, err := safeDecodeInterface(dec, len(raw))
		if err != nil {
			return Info{}, fmt.Errorf("discovery: decode value %d: %w", i, err)
		}
		switch keyByte {
		case FieldName:
			if s, ok := raw.(string); ok {
				out.Name = s
			}
		case FieldTransportID:
			switch v := raw.(type) {
			case []byte:
				out.TransportID = append([]byte(nil), v...)
			case string:
				out.TransportID = []byte(v)
			}
		case FieldInterfaceType:
			if s, ok := raw.(string); ok {
				out.Type = s
			}
		case FieldTransport:
			if b, ok := raw.(bool); ok {
				out.Transport = b
			}
		case FieldReachableOn:
			if s, ok := raw.(string); ok {
				out.ReachableOn = s
			}
		case FieldLatitude:
			out.Latitude = toFloat(raw)
			out.HasGeo = true
		case FieldLongitude:
			out.Longitude = toFloat(raw)
			out.HasGeo = true
		case FieldHeight:
			out.Height = toFloat(raw)
			out.HasGeo = true
		case FieldPort:
			out.Port = toInt(raw)
			out.HasPort = true
		case FieldIFACNetname:
			if s, ok := raw.(string); ok {
				out.IFACNetname = s
			}
		case FieldIFACNetkey:
			if s, ok := raw.(string); ok {
				out.IFACNetkey = s
			}
		case FieldFrequency:
			out.Frequency = toInt(raw)
		case FieldBandwidth:
			out.Bandwidth = toInt(raw)
		case FieldSpreadingFactor:
			out.SpreadingFactor = toInt(raw)
		case FieldCodingRate:
			out.CodingRate = toInt(raw)
		case FieldChannel:
			out.Channel = toInt(raw)
		case FieldModulation:
			if s, ok := raw.(string); ok {
				out.Modulation = s
			}
		}
	}
	return out, nil
}

// EncodeAppData wraps an info payload in the flag-byte+payload+stamp format
// used as the discovery announce app_data in Discovery. payload is the raw
// msgpack info bytes returned by EncodeInfo, stamp is the LXStamper output
// (32 bytes), and flags is the OR of the FlagSigned / FlagEncrypted bits.
func EncodeAppData(flags byte, payload, stamp []byte) ([]byte, error) {
	if len(stamp) != StampSize {
		return nil, fmt.Errorf("discovery: stamp must be %d bytes, got %d", StampSize, len(stamp))
	}
	out := make([]byte, 0, 1+len(payload)+StampSize)
	out = append(out, flags)
	out = append(out, payload...)
	out = append(out, stamp...)
	return out, nil
}

// DecodeAppData reverses EncodeAppData, returning the flags, info payload
// and stamp from a raw discovery announce app_data buffer.
func DecodeAppData(raw []byte) (flags byte, payload, stamp []byte, err error) {
	if len(raw) <= 1+StampSize {
		return 0, nil, nil, fmt.Errorf("discovery: app_data too short (%d bytes)", len(raw))
	}
	if len(raw) > MaxAppDataSize {
		return 0, nil, nil, fmt.Errorf("discovery: app_data too large (%d > %d)", len(raw), MaxAppDataSize)
	}
	flags = raw[0]
	payload = raw[1 : len(raw)-StampSize]
	stamp = raw[len(raw)-StampSize:]
	return flags, payload, stamp, nil
}

// safeDecodeInterface decodes a single msgpack value, refusing types whose
// declared collection length could trigger an unbounded allocation in the
// underlying msgpack decoder. Composite types (arrays, maps, ext) are
// rejected; only primitives, strings and binary blobs (bounded by max)
// are accepted.
func safeDecodeInterface(dec *msgpack.Decoder, max int) (any, error) {
	c, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	switch {
	case c == msgpcode.Nil:
		_ = dec.DecodeNil()
		return nil, nil
	case c == msgpcode.True || c == msgpcode.False:
		return dec.DecodeBool()
	case c == msgpcode.Float:
		f, err := dec.DecodeFloat32()
		return float64(f), err
	case c == msgpcode.Double:
		return dec.DecodeFloat64()
	case c == msgpcode.Int8 || c == msgpcode.Int16 || c == msgpcode.Int32 || c == msgpcode.Int64:
		return dec.DecodeInt64()
	case c == msgpcode.Uint8 || c == msgpcode.Uint16 || c == msgpcode.Uint32 || c == msgpcode.Uint64:
		return dec.DecodeUint64()
	case c <= msgpcode.PosFixedNumHigh:
		return dec.DecodeInt64()
	case c >= msgpcode.NegFixedNumLow:
		return dec.DecodeInt64()
	}
	isStr := (c >= msgpcode.FixedStrLow && c <= msgpcode.FixedStrHigh) ||
		c == msgpcode.Str8 || c == msgpcode.Str16 || c == msgpcode.Str32
	isBin := c == msgpcode.Bin8 || c == msgpcode.Bin16 || c == msgpcode.Bin32
	if isStr || isBin {
		b, err := safeDecodeBytes(dec, max)
		if err != nil {
			return nil, err
		}
		if isStr {
			return string(b), nil
		}
		return b, nil
	}
	return nil, fmt.Errorf("discovery: unsupported msgpack code 0x%02x in info value", c)
}

// safeDecodeBytes reads a bin/str length and rejects anything whose
// declared length exceeds the available payload before allocating.
func safeDecodeBytes(dec *msgpack.Decoder, max int) ([]byte, error) {
	n, err := dec.DecodeBytesLen()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	if n > max {
		return nil, fmt.Errorf("discovery: declared bin/str length %d exceeds payload size %d", n, max)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(dec.Buffered(), b); err != nil {
		return nil, fmt.Errorf("discovery: read bin/str payload: %w", err)
	}
	return b, nil
}

// StampWorkblock builds the workblock used by stamp_value and stamp_valid.
// Equivalent to LXMF.LXStamper.stamp_workblock.
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		expandRounds = WorkblockExpandRounds
	}
	out := make([]byte, 0, 256*expandRounds)
	for n := 0; n < expandRounds; n++ {
		nPacked, err := msgpack.Marshal(n)
		if err != nil {
			return nil, fmt.Errorf("discovery: encode round %d: %w", n, err)
		}
		salt := cryptography.Hash(append(append([]byte(nil), material...), nPacked...))
		block, err := cryptography.DeriveKey(material, salt, nil, 256)
		if err != nil {
			return nil, fmt.Errorf("discovery: hkdf round %d: %w", n, err)
		}
		out = append(out, block...)
	}
	return out, nil
}

// StampValue counts the leading-zero bits of sha256(workblock || stamp),
// matching LXMF.LXStamper.stamp_value.
func StampValue(workblock, stamp []byte) int {
	h := sha256.Sum256(append(append([]byte(nil), workblock...), stamp...))
	value := 0
	for _, b := range h {
		if b == 0 {
			value += 8
			continue
		}
		for mask := byte(0x80); mask != 0; mask >>= 1 {
			if b&mask == 0 {
				value++
			} else {
				return value
			}
		}
	}
	return value
}

// StampValid reports whether stamp meets targetCost on the given workblock.
// Equivalent to LXMF.LXStamper.stamp_valid.
func StampValid(stamp []byte, targetCost int, workblock []byte) bool {
	if targetCost < 0 || targetCost > 256 {
		return false
	}
	return StampValue(workblock, stamp) >= targetCost
}

// GenerateStamp brute-forces a 32-byte stamp such that
// StampValue(workblock, stamp) >= stampCost. Matches
// LXMF.LXStamper.generate_stamp's job_simple variant. The workblock is
// derived from messageID with the same expand rounds the verifier will use
// (defaulting to WorkblockExpandRounds, the value Discovery uses).
func GenerateStamp(messageID []byte, stampCost int, expandRounds int) (stamp []byte, value int, err error) {
	workblock, err := StampWorkblock(messageID, expandRounds)
	if err != nil {
		return nil, 0, err
	}
	stamp = make([]byte, StampSize)
	for {
		if _, err := rand.Read(stamp); err != nil {
			return nil, 0, fmt.Errorf("discovery: read random stamp: %w", err)
		}
		if StampValid(stamp, stampCost, workblock) {
			return stamp, StampValue(workblock, stamp), nil
		}
	}
}

// InfoHash returns sha256(packedInfo), the message_id passed to
// LXStamper.generate_stamp by Discovery for an interface announce.
func InfoHash(packed []byte) []byte {
	h := sha256.Sum256(packed)
	return h[:]
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	}
	return 0
}

func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case uint64:
		// Clamp to int64 max instead of allowing a sign-flip on values
		// above 2^63-1; numbers that big are not produced by conforming peers but
		// could appear in a hostile or fuzzed payload.
		const maxInt64 = uint64(1<<63 - 1)
		if x > maxInt64 {
			return int64(maxInt64) //nolint:gosec // G115: guarded above
		}
		return int64(x) //nolint:gosec // G115: guarded above
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}
