// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:encoding/hex"
import "core:strings"

cstring_field :: proc(buf: []u8) -> string {
	n := 0
	for b in buf {
		if b == 0 {
			break
		}
		n += 1
	}
	return string(buf[:n])
}

hash_to_hex :: proc(hash: []u8, allocator := context.allocator) -> (hex_str: string, ok: bool) {
	encoded, err := hex.encode(hash, allocator)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

hex_to_hash :: proc(s: string, allocator := context.allocator) -> (hash: []u8, ok: bool) {
	return hex.decode(transmute([]byte)s, allocator)
}

clone_cstring_slice :: proc(values: []string, allocator := context.temp_allocator) -> []cstring {
	out := make([]cstring, len(values), allocator)
	for v, i in values {
		out[i] = strings.clone_to_cstring(v, allocator)
	}
	return out
}
