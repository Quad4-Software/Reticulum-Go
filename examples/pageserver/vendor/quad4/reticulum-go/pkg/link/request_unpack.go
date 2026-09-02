// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"errors"
	"fmt"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
)

func unpackLinkRequest(plaintext []byte) (requestedAt time.Time, pathHash []byte, requestPayload []byte, err error) {
	var parts []msgpack.RawMessage
	if err := msgpack.Unmarshal(plaintext, &parts); err != nil {
		return time.Time{}, nil, nil, fmt.Errorf("failed to unpack request: %w", err)
	}
	if len(parts) < MinRequestDataLen {
		return time.Time{}, nil, nil, errors.New("invalid request format")
	}
	var ts any
	if err := msgpack.Unmarshal(parts[0], &ts); err != nil {
		return time.Time{}, nil, nil, fmt.Errorf("invalid requested_at: %w", err)
	}
	requestedAt, err = parseRequestedAt(ts)
	if err != nil {
		return time.Time{}, nil, nil, err
	}
	if err := msgpack.Unmarshal(parts[1], &pathHash); err != nil {
		return time.Time{}, nil, nil, fmt.Errorf("invalid path_hash: %w", err)
	}
	if len(parts[2]) > 0 {
		var decoded any
		if err := msgpack.Unmarshal(parts[2], &decoded); err != nil {
			requestPayload = append([]byte(nil), parts[2]...)
		} else {
			requestPayload, err = linkRequestPayload(decoded, parts[2])
			if err != nil {
				return time.Time{}, nil, nil, err
			}
		}
	}
	return requestedAt, pathHash, requestPayload, nil
}

func linkRequestPayload(decoded any, raw []byte) ([]byte, error) {
	switch v := decoded.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return append([]byte(nil), raw...), nil
	}
}
