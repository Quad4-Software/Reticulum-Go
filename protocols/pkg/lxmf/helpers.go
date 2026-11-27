// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack"
	"git.quad4.io/libraries/msgpack/v5/pkg/msgpack/msgpcode"
)

// DisplayNameFromAppData returns the announce display name (legacy UTF-8 or v0.5.0+ msgpack).
func DisplayNameFromAppData(appData []byte) (string, error) {
	if len(appData) == 0 {
		return "", nil
	}
	if len(appData) > maxAnnounceAppDataBytes {
		return "", errors.New("lxmf: announce app data too large")
	}

	if isMsgpackArray(appData[0]) {
		arr, err := unmarshalMsgpackArray(appData)
		if err != nil {
			return "", err
		}
		if len(arr) == 0 {
			return "", nil
		}
		switch v := arr[0].(type) {
		case nil:
			return "", nil
		case []byte:
			return sanitizeAnnounceDisplayName(v), nil
		case string:
			return sanitizeAnnounceDisplayName([]byte(v)), nil
		default:
			return "", errors.New("lxmf: unsupported display name type")
		}
	}

	return string(appData), nil
}

// sanitizeAnnounceDisplayName matches upstream v0.5.0+ announce display name handling.
func sanitizeAnnounceDisplayName(raw []byte) string {
	return strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", ""))
}

const announceAppDataArrayMax = 32

// maxAnnounceAppDataBytes limits announce decoding input (DoS bound for msgpack).
const maxAnnounceAppDataBytes = 8192

// unmarshalMsgpackArray decodes a top-level array with the same map limits as message payloads.
func unmarshalMsgpackArray(appData []byte) ([]any, error) {
	dec := msgpack.NewDecoder(bytes.NewReader(appData))
	dec.UseLooseInterfaceDecoding(true)
	mapCtx := &msgpackMapCtx{maxDepth: msgpackMapMaxDepth, maxPairs: msgpackMapMaxPairs}
	dec.SetMapDecoder(mapCtx.decodeMap)
	n, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, err
	}
	if n == -1 {
		return nil, nil
	}
	if n > announceAppDataArrayMax {
		return nil, errors.New("lxmf: announce app data array too long")
	}
	out := make([]any, 0, n)
	for range n {
		v, err := dec.DecodeInterface()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// StampCostFromAppData returns the v0.5.0+ stamp cost. Ok false for legacy UTF-8 announces.
func StampCostFromAppData(appData []byte) (cost int64, ok bool, err error) {
	if len(appData) == 0 {
		return 0, false, nil
	}
	if len(appData) > maxAnnounceAppDataBytes {
		return 0, false, errors.New("lxmf: announce app data too large")
	}
	if !isMsgpackArray(appData[0]) {
		return 0, false, nil
	}

	arr, err := unmarshalMsgpackArray(appData)
	if err != nil {
		return 0, false, err
	}
	if len(arr) < 2 {
		return 0, false, nil
	}
	switch v := arr[1].(type) {
	case nil:
		return 0, false, nil
	case int8:
		return clampAnnounceStampCost(int64(v))
	case int16:
		return clampAnnounceStampCost(int64(v))
	case int32:
		return clampAnnounceStampCost(int64(v))
	case int64:
		return clampAnnounceStampCost(v)
	case uint8:
		return clampAnnounceStampCost(int64(v))
	case uint16:
		return clampAnnounceStampCost(int64(v))
	case uint32:
		return clampAnnounceStampCost(int64(v))
	case uint64:
		const maxStampCost uint64 = 1<<63 - 1
		if v > maxStampCost {
			return 0, false, nil
		}
		return clampAnnounceStampCost(int64(v))
	default:
		return 0, false, nil
	}
}

func clampAnnounceStampCost(cost int64) (int64, bool, error) {
	if cost < 0 {
		return 0, false, nil
	}
	return cost, true, nil
}

// EncodeAnnounceAppData returns legacy announce app data as raw UTF-8.
func EncodeAnnounceAppData(displayName string) []byte {
	return []byte(displayName)
}

// EncodeAnnounceAppDataV5 builds v0.5.0+ [name, cost?] without a feature list (implies all SF flags).
func EncodeAnnounceAppDataV5(displayName string, stampCost int64) ([]byte, error) {
	var costElement any
	if stampCost >= 0 {
		costElement = stampCost
	}
	payload := []any{[]byte(displayName), costElement}
	return marshalAnnounceAppData(payload)
}

// EncodeAnnounceAppDataV5WithFeatures builds v0.5.0+ announce data with optional [SF] list.
// Nil features omits the third element (same as EncodeAnnounceAppDataV5). Non-nil empty slice encodes [].
func EncodeAnnounceAppDataV5WithFeatures(displayName string, stampCost int64, features []byte) ([]byte, error) {
	var costElement any
	if stampCost >= 0 {
		costElement = stampCost
	}
	payload := []any{[]byte(displayName), costElement}
	if features != nil {
		fl := make([]any, 0, len(features))
		for _, f := range features {
			fl = append(fl, f)
		}
		payload = append(payload, fl)
	}
	return marshalAnnounceAppData(payload)
}

// EncodeAnnounceAppDataWithIcon builds v0.5.0+ announce app data with optional icon appearance.
// Layout is [name, cost?, [icon_name, fg_rgb3, bg_rgb3]?]. Empty iconName omits the icon element.
func EncodeAnnounceAppDataWithIcon(displayName string, stampCost int64, iconName string, fgRGB, bgRGB []byte) ([]byte, error) {
	var costElement any
	if stampCost >= 0 {
		costElement = stampCost
	}
	payload := []any{[]byte(displayName), costElement}
	name := strings.TrimSpace(iconName)
	if name != "" {
		fg := normalizeAnnounceRGB3(fgRGB)
		bg := normalizeAnnounceRGB3(bgRGB)
		payload = append(payload, []any{name, fg, bg})
	}
	return marshalAnnounceAppData(payload)
}

func normalizeAnnounceRGB3(rgb []byte) []byte {
	out := []byte{0, 0, 0}
	if len(rgb) >= 3 {
		copy(out, rgb[:3])
	}
	return out
}

// EncodePNAnnounceAppData builds the 7-element propagation node announce
// payload. nodeState matches upstream propagation_node and not
// from_static_only. The limits are announced as configured either way.
func EncodePNAnnounceAppData(timebase int64, nodeState bool, transferLimitKB, syncLimitKB int, stampTarget, stampFlex, peeringCost int, nodeName string) ([]byte, error) {
	// Upstream lxmd 1.3.0 always announces implementation name and
	// version in the propagation node metadata.
	md := map[byte]any{
		PNMetaImplName: "golxmd",
		PNMetaVersion:  Version,
	}
	if nodeName != "" {
		md[PNMetaName] = []byte(nodeName)
	}
	payload := []any{
		nil,
		timebase,
		nodeState,
		int64(transferLimitKB),
		int64(syncLimitKB),
		[]any{int64(stampTarget), int64(stampFlex), int64(peeringCost)},
		md,
	}
	return marshalAnnounceAppData(payload)
}

func marshalAnnounceAppData(payload []any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.UseCompactInts(true)
	if err := enc.Encode(payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CompressionSupportFromAppData reports bz2 Resource support per upstream rules.
// ok is false for empty input. Legacy and short v0.5.0+ arrays default to supported true.
func CompressionSupportFromAppData(appData []byte) (supported, ok bool, err error) {
	if len(appData) == 0 {
		return false, false, nil
	}
	if len(appData) > maxAnnounceAppDataBytes {
		return false, false, errors.New("lxmf: announce app data too large")
	}
	if !isMsgpackArray(appData[0]) {
		return true, true, nil
	}

	arr, err := unmarshalMsgpackArray(appData)
	if err != nil {
		return false, false, err
	}
	if len(arr) < 3 {
		return true, true, nil
	}
	codes, isList := arr[2].([]any)
	if !isList {
		return true, true, nil
	}
	for _, c := range codes {
		v, asInt := asInt64(c)
		if !asInt {
			continue
		}
		if v >= 0 && v <= 0xFF && byte(v) == SFCompression {
			return true, true, nil
		}
	}
	return false, true, nil
}

// FeatureSupportFromAppData returns the third-element SF code list when present. Ok false if omitted or legacy.
func FeatureSupportFromAppData(appData []byte) (codes []byte, ok bool, err error) {
	if len(appData) == 0 {
		return nil, false, nil
	}
	if len(appData) > maxAnnounceAppDataBytes {
		return nil, false, errors.New("lxmf: announce app data too large")
	}
	if !isMsgpackArray(appData[0]) {
		return nil, false, nil
	}

	arr, err := unmarshalMsgpackArray(appData)
	if err != nil {
		return nil, false, err
	}
	if len(arr) < 3 {
		return nil, false, nil
	}
	raw, isList := arr[2].([]any)
	if !isList {
		return nil, false, nil
	}
	out := make([]byte, 0, len(raw))
	for _, c := range raw {
		v, asInt := asInt64(c)
		if !asInt {
			continue
		}
		if v < 0 || v > 0xFF {
			continue
		}
		out = append(out, byte(v))
	}
	return out, true, nil
}

func isMsgpackArray(prefix byte) bool {
	return (prefix >= 0x90 && prefix <= 0x9f) || prefix == 0xdc || prefix == 0xdd
}

// PNAnnounceDataIsValid reports whether data matches the upstream 7-element PN announce layout.
func PNAnnounceDataIsValid(data []byte) bool {
	arr, err := decodePNAnnounceArray(data)
	if err != nil {
		return false
	}
	if len(arr) < 7 {
		return false
	}
	if _, ok := asInt64(arr[1]); !ok {
		return false
	}
	if _, ok := arr[2].(bool); !ok {
		return false
	}
	if _, ok := asInt64(arr[3]); !ok {
		return false
	}
	if _, ok := asInt64(arr[4]); !ok {
		return false
	}
	costs, ok := arr[5].([]any)
	if !ok || len(costs) < 3 {
		return false
	}
	for _, c := range costs[:3] {
		if _, ok := asInt64(c); !ok {
			return false
		}
	}
	if _, ok := arr[6].(map[any]any); !ok {
		if _, ok := arr[6].(map[string]any); !ok {
			return false
		}
	}
	return true
}

// PNStampCostFromAppData returns PN target stamp cost from a valid PN announce.
func PNStampCostFromAppData(data []byte) (int64, bool, error) {
	arr, err := decodePNAnnounceArray(data)
	if err != nil {
		return 0, false, err
	}
	if !pnAnnounceShapeValid(arr) {
		return 0, false, nil
	}
	costs, _ := arr[5].([]any)
	cost, ok := asInt64(costs[0])
	if !ok || cost < 0 {
		return 0, false, nil
	}
	return cost, true, nil
}

// PNNameFromAppData returns PNMetaName from PN announce metadata.
func PNNameFromAppData(data []byte) (string, bool, error) {
	arr, err := decodePNAnnounceArray(data)
	if err != nil {
		return "", false, err
	}
	if !pnAnnounceShapeValid(arr) {
		return "", false, nil
	}
	switch md := arr[6].(type) {
	case map[any]any:
		v, ok := lookupAnyKey(md, PNMetaName)
		if !ok {
			return "", false, nil
		}
		return decodePNNameValue(v)
	case map[string]any:
		v, ok := md[string([]byte{PNMetaName})]
		if !ok {
			return "", false, nil
		}
		return decodePNNameValue(v)
	}
	return "", false, nil
}

func decodePNAnnounceArray(data []byte) ([]any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	const maxPNAnnounceBytes = 256 << 10
	if len(data) > maxPNAnnounceBytes {
		return nil, errors.New("lxmf: propagation announce data too large")
	}
	dec := msgpack.NewDecoder(bytes.NewReader(data))
	dec.UseLooseInterfaceDecoding(true)
	mapCtx := &msgpackMapCtx{maxDepth: msgpackMapMaxDepth, maxPairs: msgpackMapMaxPairs}
	dec.SetMapDecoder(mapCtx.decodeMap)

	n, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, err
	}
	// Upstream accepts announces with at least 7 fields and ignores
	// any trailing extension fields.
	if n < 7 {
		return nil, nil
	}

	out := make([]any, 0, 7)

	code0, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	if code0 == msgpcode.Nil {
		if err := dec.DecodeNil(); err != nil {
			return nil, err
		}
		out = append(out, nil)
	} else {
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		out = append(out, nil)
	}

	v1, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	out = append(out, v1)

	v2, err := dec.DecodeBool()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	out = append(out, v2)

	v3, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	out = append(out, v3)

	v4, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	out = append(out, v4)

	na, err := dec.DecodeArrayLen()
	if err != nil {
		return nil, err
	}
	if na < 3 {
		return nil, nil
	}
	ca0, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	ca1, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	ca2, err := dec.DecodeInt64()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return nil, nil
	}
	for i := 3; i < na; i++ {
		if err := dec.Skip(); err != nil {
			return nil, err
		}
	}
	out = append(out, []any{ca0, ca1, ca2})

	v6, err := dec.DecodeInterface()
	if err != nil {
		return nil, err
	}
	out = append(out, v6)

	for i := 7; i < n; i++ {
		if err := dec.Skip(); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func pnAnnounceShapeValid(arr []any) bool {
	if len(arr) < 7 {
		return false
	}
	costs, ok := arr[5].([]any)
	if !ok || len(costs) < 3 {
		return false
	}
	if _, ok := arr[6].(map[any]any); ok {
		return true
	}
	if _, ok := arr[6].(map[string]any); ok {
		return true
	}
	return false
}

func lookupAnyKey(m map[any]any, key byte) (any, bool) {
	for k, v := range m {
		ki, ok := asInt64(k)
		if !ok {
			continue
		}
		if ki == int64(key) {
			return v, true
		}
	}
	return nil, false
}

func decodePNNameValue(v any) (string, bool, error) {
	switch x := v.(type) {
	case nil:
		return "", false, nil
	case []byte:
		return string(x), true, nil
	case string:
		return x, true, nil
	default:
		return "", false, nil
	}
}

func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case uint:
		const maxInt64AsUint = ^uint(0) >> 1
		if x > maxInt64AsUint {
			return 0, false
		}
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		const maxInt64 uint64 = 1<<63 - 1
		if x > maxInt64 {
			return 0, false
		}
		return int64(x), true
	case float32:
		f := float64(x)
		if f == float64(int64(f)) {
			return int64(f), true
		}
	case float64:
		if x == float64(int64(x)) {
			return int64(x), true
		}
	}
	return 0, false
}
