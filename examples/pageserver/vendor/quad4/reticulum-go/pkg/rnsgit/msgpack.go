// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rnsgit

import (
	"fmt"
	"math"

	"quad4/msgpack/v5/pkg/msgpack"
)

// EncodeRequest packs a rngit request dict with integer keys.
func EncodeRequest(fields map[int]any) ([]byte, error) {
	if len(fields) == 0 {
		return msgpack.Marshal(map[int]any{})
	}
	return msgpack.Marshal(fields)
}

// EncodeMixedRequest packs a rngit request dict with mixed integer and string keys.
func EncodeMixedRequest(fields map[any]any) ([]byte, error) {
	return msgpack.Marshal(fields)
}

// DecodeRequest unpacks a rngit request dict.
func DecodeRequest(data []byte) (map[any]any, error) {
	if len(data) == 0 {
		return map[any]any{}, nil
	}
	var raw map[any]any
	if err := msgpack.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// RepoFromRequest returns the repository path from a decoded request.
func RepoFromRequest(req map[any]any) (string, bool) {
	for k, v := range req {
		ik, ok := toIntKey(k)
		if !ok || ik != IdxRepository {
			continue
		}
		switch s := v.(type) {
		case string:
			return s, s != ""
		case []byte:
			return string(s), len(s) > 0
		default:
			return fmt.Sprint(v), true
		}
	}
	return "", false
}

// PermsGetResponse builds a rngit-compatible permissions get payload.
func PermsGetResponse(content string) []byte {
	packed, _ := msgpack.Marshal(map[string]string{"content": content})
	return append([]byte{ResOK}, packed...)
}

// ParsePermsGetBody extracts permission file text from a get response body.
func ParsePermsGetBody(body []byte) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("empty response")
	}
	if body[0] != ResOK {
		msg := string(body[1:])
		if msg == "" {
			msg = "permission request failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	if len(body) == 1 {
		return "", nil
	}
	payload := body[1:]
	var wrapped map[string]any
	if err := msgpack.Unmarshal(payload, &wrapped); err == nil {
		if c, ok := wrapped["content"].(string); ok {
			return c, nil
		}
		if c, ok := wrapped["content"].([]byte); ok {
			return string(c), nil
		}
	}
	return string(payload), nil
}

// OKMetadataPacked returns msgpack metadata with result code zero.
func OKMetadataPacked() []byte {
	b, _ := msgpack.Marshal(map[int]int{IdxResultCode: ResOK})
	return b
}

// StatusResponse builds a one-byte status plus message.
func StatusResponse(code byte, msg string) []byte {
	out := make([]byte, 1+len(msg))
	out[0] = code
	copy(out[1:], []byte(msg))
	return out
}

// MetadataResultCode reads IDX_RESULT_CODE from resource metadata.
func MetadataResultCode(meta map[string]any) (byte, bool) {
	if meta == nil {
		return 0, false
	}
	for _, key := range []string{"1", "\x01"} {
		if v, ok := meta[key]; ok {
			return byteFromAny(v)
		}
	}
	if v, ok := meta["result_code"]; ok {
		return byteFromAny(v)
	}
	return 0, false
}

func byteFromAny(v any) (byte, bool) {
	switch x := v.(type) {
	case uint8:
		return x, true
	case int8:
		if x < 0 {
			return 0, false
		}
		return byte(x), true
	case int:
		if x < 0 || x > math.MaxUint8 {
			return 0, false
		}
		return byte(x), true
	case int64:
		if x < 0 || x > math.MaxUint8 {
			return 0, false
		}
		return byte(x), true
	case uint64:
		if x > math.MaxUint8 {
			return 0, false
		}
		return byte(x), true
	default:
		return 0, false
	}
}

// MetadataResultCodeRaw unpacks integer-key metadata from raw msgpack bytes.
func MetadataResultCodeRaw(packed []byte) (byte, bool) {
	var m map[int]int
	if err := msgpack.Unmarshal(packed, &m); err == nil {
		if v, ok := m[IdxResultCode]; ok {
			if b, ok := byteFromAny(v); ok {
				return b, true
			}
		}
	}
	var anyMap map[any]any
	if err := msgpack.Unmarshal(packed, &anyMap); err != nil {
		return 0, false
	}
	for k, v := range anyMap {
		ik, ok := toIntKey(k)
		if !ok || ik != IdxResultCode {
			continue
		}
		return byteFromAny(v)
	}
	return 0, false
}

func toIntKey(k any) (int, bool) {
	switch x := k.(type) {
	case int:
		return x, true
	case int8:
		return int(x), true
	case int16:
		return int(x), true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case uint8:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case uint64:
		if x > math.MaxInt {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}
