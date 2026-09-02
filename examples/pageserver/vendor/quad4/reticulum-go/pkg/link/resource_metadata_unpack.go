// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"fmt"
	"math"
	"strconv"

	"quad4/msgpack/v5/pkg/msgpack"
)

func unpackResourceMetadata(packed []byte) (meta map[string]any, err error) {
	defer func() {
		if recover() != nil {
			meta = nil
			err = fmt.Errorf("invalid metadata msgpack")
		}
	}()
	var raw map[any]any
	if err := msgpack.Unmarshal(packed, &raw); err == nil && len(raw) > 0 {
		return normalizeMetadataMap(raw), nil
	}
	var strMap map[string]any
	if err := msgpack.Unmarshal(packed, &strMap); err != nil {
		return nil, err
	}
	return strMap, nil
}

func normalizeMetadataMap(raw map[any]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[metadataMapKey(k)] = v
	}
	return out
}

func metadataMapKey(k any) string {
	switch x := k.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.Itoa(int(x))
	case int16:
		return strconv.Itoa(int(x))
	case int32:
		return strconv.Itoa(int(x))
	case int64:
		return strconv.Itoa(int(x))
	case uint8:
		return strconv.Itoa(int(x))
	case uint16:
		return strconv.Itoa(int(x))
	case uint32:
		return strconv.Itoa(int(x))
	case uint64:
		if x > math.MaxInt {
			return fmt.Sprint(k)
		}
		return strconv.Itoa(int(x))
	default:
		return fmt.Sprint(k)
	}
}
