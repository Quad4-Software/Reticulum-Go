// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package common

import (
	"github.com/shamaton/msgpack/v2"
)

// Marshal returns the MessagePack encoding of v.
func MsgpackMarshal(v interface{}) ([]byte, error) {
	return msgpack.Marshal(v)
}

// Unmarshal parses the MessagePack-encoded data and stores the result in the value pointed to by v.
func MsgpackUnmarshal(data []byte, v interface{}) error {
	return msgpack.Unmarshal(data, v)
}
