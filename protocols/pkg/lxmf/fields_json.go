// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FieldsFromHarnessJSON decodes harness-style field JSON into a native fields map.
func FieldsFromHarnessJSON(data []byte) (map[byte]any, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[byte]any, len(raw))
	for k, v := range raw {
		key, err := fieldKeyFromString(k)
		if err != nil {
			return nil, err
		}
		normalized, err := harnessJSONValue(v)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", k, err)
		}
		out[key] = normalized
	}
	return out, nil
}

// FieldsToHarnessJSON encodes fields using harness-style hex prefixes and 0x keys.
func FieldsToHarnessJSON(fields map[byte]any) ([]byte, error) {
	if len(fields) == 0 {
		return []byte("{}"), nil
	}
	raw := make(map[string]any, len(fields))
	for k, v := range fields {
		raw[fmt.Sprintf("0x%02x", k)] = harnessOutValue(v)
	}
	return json.Marshal(raw)
}

func harnessJSONValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if strings.HasPrefix(x, "hex:") {
			b, err := hex.DecodeString(x[4:])
			if err != nil {
				return nil, err
			}
			return b, nil
		}
		return x, nil
	case float64:
		return x, nil
	case bool:
		return x, nil
	case []any:
		out := make([]any, len(x))
		for i, elem := range x {
			normalized, err := harnessJSONValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	case map[string]any:
		out := make(map[byte]any, len(x))
		for k, val := range x {
			key, err := fieldKeyFromString(k)
			if err != nil {
				return nil, err
			}
			normalized, err := harnessJSONValue(val)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported harness json value %T", v)
	}
}

func harnessOutValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return "hex:" + hex.EncodeToString(x)
	case map[byte]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprintf("0x%02x", k)] = harnessOutValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, elem := range x {
			out[i] = harnessOutValue(elem)
		}
		return out
	case float32:
		return float64(x)
	case int:
		return x
	case int64:
		return x
	case uint8:
		return int(x)
	case string:
		return x
	case bool:
		return x
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	default:
		return fmt.Sprint(x)
	}
}

// HarnessFieldKeys returns sorted 0xNN keys present in a harness JSON fields object.
func HarnessFieldKeys(data []byte) ([]string, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, strings.ToLower(k))
	}
	sortStrings(keys)
	return keys, nil
}

func sortStrings(in []string) {
	for i := range in {
		for j := i + 1; j < len(in); j++ {
			ai, _ := strconv.ParseInt(strings.TrimPrefix(in[i], "0x"), 16, 32)
			aj, _ := strconv.ParseInt(strings.TrimPrefix(in[j], "0x"), 16, 32)
			if aj < ai {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}
