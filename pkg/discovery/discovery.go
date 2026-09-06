// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package discovery

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/msgpack/v5/pkg/msgpack/msgpcode"
	"quad4/reticulum-go/pkg/lxstamper"
)

// AppName is the destination app_name used by Discovery (see const value).
const AppName = "rnstransport"

// Aspects is the destination aspect chain used by Discovery
// (discovery.interface).
var Aspects = []string{"discovery", "interface"}

// ImplementationName is the short transport stack id announced in discovery
// info (RNS 1.5.1+ TRANSPORT_IMPL). Python RNS uses "RNS".
const ImplementationName = "reticulum-go"

// Field tags from Discovery. Each is a single-byte msgpack map key.
const (
	FieldName            byte = 0xFF
	FieldTransportID     byte = 0xFE
	FieldTransportImpl   byte = 0xFD
	FieldTransportVers   byte = 0xFC
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
	FieldOpAddr          byte = 0xF0
	// FieldOpPage is a provisional operator NomadNet page destination hash.
	// Upstream RNS has not assigned this tag yet. Python ignores unknown map
	// keys, so emitting it does not break receive-side interop. Remap if RNS
	// standardizes a different code.
	FieldOpPage byte = 0xF1
)

// implementationVersion is the fallback TRANSPORT_VERS when build info is
// unavailable. Link with -X if a pinned release string is required.
var implementationVersion = "dev"

// ImplementationVersion returns the transport version string for discovery
// announces (RNS 1.5.1+ TRANSPORT_VERS).
func ImplementationVersion() string {
	if implementationVersion != "" && implementationVersion != "dev" {
		return implementationVersion
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	if implementationVersion == "" {
		return "dev"
	}
	return implementationVersion
}

// Flag bits used in the announce app_data flag byte.
const (
	FlagSigned    byte = 0b00000001
	FlagEncrypted byte = 0b00000010
)

// DefaultStampValue is the default proof-of-work target value applied when
// stamping discovery announcements.
const DefaultStampValue = 16

// WorkblockExpandRounds controls the HKDF expansion rounds used to derive the
// stamp workblock for discovery announcements.
const WorkblockExpandRounds = lxstamper.DiscoveryRounds

// StampSize is the size in bytes of a discovery proof-of-work stamp
// (one identity hash).
const StampSize = lxstamper.StampSize

// Info is the high-level Go representation of a discovery info payload. Only
// fields that were present in the msgpack map are populated. Check the

// corresponding Has* boolean before reading optional numeric fields.
type Info struct {
	Type        string
	Transport   bool
	TransportID []byte
	// TransportImpl is RNS 1.5.1+ TRANSPORT_IMPL (empty uses ImplementationName on encode).
	TransportImpl string
	// TransportVers is RNS 1.5.1+ TRANSPORT_VERS (empty uses ImplementationVersion on encode).
	TransportVers string
	Name          string
	Latitude      float64
	Longitude     float64
	Height        float64
	HasGeo        bool

	ReachableOn string
	Port        int64
	HasPort     bool

	IFACNetname string
	IFACNetkey  string

	Frequency           int64
	Bandwidth           int64
	SpreadingFactor     int64
	CodingRate          int64
	Channel             int64
	Modulation          string
	OperatorLXMFAddress []byte
	OperatorPageAddress []byte
}

// EncodeInfo serialises an Info into the msgpack representation used as the
// info dictionary inside an interface announce app_data payload. Numeric
// fields are omitted when zero unless the matching Has flag is set. Callers

// that want explicit zero-valued fields should set the appropriate flag.
func EncodeInfo(in Info) ([]byte, error) {
	if in.Type == "" {
		return nil, errors.New("discovery: Info.Type required")
	}
	if len(in.TransportID) == 0 {
		return nil, errors.New("discovery: Info.TransportID required")
	}

	impl := in.TransportImpl
	if impl == "" {
		impl = ImplementationName
	}
	vers := in.TransportVers
	if vers == "" {
		vers = ImplementationVersion()
	}

	pairs := make([][2]any, 0, 18)
	pairs = append(pairs,
		[2]any{FieldInterfaceType, in.Type},
		[2]any{FieldTransport, in.Transport},
		[2]any{FieldTransportID, in.TransportID},
		[2]any{FieldTransportImpl, impl},
		[2]any{FieldTransportVers, vers},
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
	if len(in.OperatorLXMFAddress) == 16 {
		pairs = append(pairs, [2]any{FieldOpAddr, in.OperatorLXMFAddress})
	}
	if len(in.OperatorPageAddress) == 16 {
		pairs = append(pairs, [2]any{FieldOpPage, in.OperatorPageAddress})
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
// Real-world Discovery info dictionaries are well below this. The cap

// defeats adversarial inputs that declare collection lengths far larger
// than the buffer they actually carry.
const MaxInfoSize = 64 * 1024

// MaxInfoFields bounds the number of fields accepted in a single info
// dictionary. Discovery uses ~16 fields. Anything wildly larger is

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
	for i := range n {
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
		case FieldTransportImpl:
			if s, ok := raw.(string); ok {
				out.TransportImpl = s
			}
		case FieldTransportVers:
			if s, ok := raw.(string); ok {
				out.TransportVers = s
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
			if v, ok := toFloatOK(raw); ok {
				out.Latitude = v
				out.HasGeo = true
			}
		case FieldLongitude:
			if v, ok := toFloatOK(raw); ok {
				out.Longitude = v
				out.HasGeo = true
			}
		case FieldHeight:
			if v, ok := toFloatOK(raw); ok {
				out.Height = v
				out.HasGeo = true
			}
		case FieldPort:
			if v, ok := toIntOK(raw); ok {
				out.Port = v
				out.HasPort = true
			}
		case FieldIFACNetname:
			if s, ok := raw.(string); ok {
				out.IFACNetname = s
			}
		case FieldIFACNetkey:
			if s, ok := raw.(string); ok {
				out.IFACNetkey = s
			}
		case FieldFrequency:
			if v, ok := toIntOK(raw); ok {
				out.Frequency = v
			}
		case FieldBandwidth:
			if v, ok := toIntOK(raw); ok {
				out.Bandwidth = v
			}
		case FieldSpreadingFactor:
			if v, ok := toIntOK(raw); ok {
				out.SpreadingFactor = v
			}
		case FieldCodingRate:
			if v, ok := toIntOK(raw); ok {
				out.CodingRate = v
			}
		case FieldChannel:
			if v, ok := toIntOK(raw); ok {
				out.Channel = v
			}
		case FieldModulation:
			if s, ok := raw.(string); ok {
				out.Modulation = s
			}
		case FieldOpAddr:
			switch v := raw.(type) {
			case []byte:
				if len(v) == 16 {
					out.OperatorLXMFAddress = append([]byte(nil), v...)
				}
			case string:
				if len(v) == 16 {
					out.OperatorLXMFAddress = []byte(v)
				}
			}
		case FieldOpPage:
			switch v := raw.(type) {
			case []byte:
				if len(v) == 16 {
					out.OperatorPageAddress = append([]byte(nil), v...)
				}
			case string:
				if len(v) == 16 {
					out.OperatorPageAddress = []byte(v)
				}
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
// rejected. Only primitives, strings and binary blobs (bounded by max)

// are accepted.
func safeDecodeInterface(dec *msgpack.Decoder, maxLen int) (any, error) {
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
		b, err := safeDecodeBytes(dec, maxLen)
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
func safeDecodeBytes(dec *msgpack.Decoder, maxLen int) ([]byte, error) {
	n, err := dec.DecodeBytesLen()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	if n > maxLen {
		return nil, fmt.Errorf("discovery: declared bin/str length %d exceeds payload size %d", n, maxLen)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(dec.Buffered(), b); err != nil {
		return nil, fmt.Errorf("discovery: read bin/str payload: %w", err)
	}
	return b, nil
}

// StampWorkblock builds the workblock used by StampValue and StampValid.
func StampWorkblock(material []byte, expandRounds int) ([]byte, error) {
	if expandRounds <= 0 {
		expandRounds = WorkblockExpandRounds
	}
	return lxstamper.StampWorkblock(material, expandRounds)
}

// StampValue counts the leading-zero bits of sha256(workblock || stamp).
func StampValue(workblock, stamp []byte) int {
	return lxstamper.StampValue(workblock, stamp)
}

// StampValid reports whether stamp meets the LXStamper numeric threshold for
// targetCost. Discovery acceptance also requires StampValue >= targetCost via
// MeetsCost / ValidateAndDecode.
func StampValid(stamp []byte, targetCost int, workblock []byte) bool {
	if targetCost < 0 || targetCost > 256 {
		return false
	}
	if len(stamp) != StampSize {
		return false
	}
	return lxstamper.StampValid(stamp, targetCost, workblock)
}

// MeetsCost reports whether stamp passes both LXStamper StampValid and
// StampValue >= targetCost, matching RNS Discovery receive gating.
func MeetsCost(stamp []byte, targetCost int, workblock []byte) bool {
	if targetCost < 0 || targetCost > 256 {
		return false
	}
	return lxstamper.MeetsCost(stamp, targetCost, workblock)
}

// GenerateStamp brute-forces a 32-byte stamp meeting the LXStamper threshold
// for stampCost. expandRounds defaults to WorkblockExpandRounds when <= 0.
// stampCost <= 0 returns a random stamp without searching (zero cost is free).
func GenerateStamp(messageID []byte, stampCost int, expandRounds int) (stamp []byte, value int, err error) {
	if expandRounds <= 0 {
		expandRounds = WorkblockExpandRounds
	}
	if stampCost <= 0 {
		stamp = make([]byte, StampSize)
		if _, err := cryptoRand.Read(stamp); err != nil {
			return nil, 0, fmt.Errorf("discovery: read random stamp: %w", err)
		}
		wb, err := StampWorkblock(messageID, expandRounds)
		if err != nil {
			return nil, 0, err
		}
		return stamp, StampValue(wb, stamp), nil
	}
	return lxstamper.GenerateStamp(context.Background(), messageID, stampCost, expandRounds)
}

// InfoHash returns sha256(packedInfo), used as the message id when stamping
// an interface announce.
func InfoHash(packed []byte) []byte {
	h := sha256.Sum256(packed)
	return h[:]
}

func toFloat(v any) float64 {
	f, _ := toFloatOK(v)
	return f
}

func toFloatOK(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint64:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	}
	return 0, false
}

func toInt(v any) int64 {
	i, _ := toIntOK(v)
	return i
}

func toIntOK(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case uint64:
		// Clamp to int64 max instead of allowing a sign-flip on values
		// above 2^63-1. Numbers that big are not produced by conforming peers but

		// could appear in a hostile or fuzzed payload.
		const maxInt64 = uint64(1<<63 - 1)
		if x > maxInt64 {
			return int64(maxInt64), true //nolint:gosec // G115: guarded above
		}
		return int64(x), true //nolint:gosec // G115: guarded above
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case float64:
		return int64(x), true
	}
	return 0, false
}
