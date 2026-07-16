// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

version :: proc() -> string {
	v := rns_version()
	if v == nil {
		return ""
	}
	return string(v)
}

last_error :: proc(allocator := context.allocator) -> (msg: string, err: Error) {
	buf: [512]u8
	written: c.size_t
	code := Error(rns_last_error(raw_data(buf[:]), len(buf), &written))
	if code != .Ok {
		return "", code
	}
	n := min(int(written), len(buf))
	return strings.clone(string(buf[:n]), allocator), .Ok
}

error_string :: proc(err: Error) -> string {
	switch err {
	case .Ok:
		return "ok"
	case .Invalid_Arg:
		return "invalid argument"
	case .Invalid_Handle:
		return "invalid handle"
	case .Not_Found:
		return "not found"
	case .State:
		return "invalid state"
	case .IO:
		return "io error"
	case .Internal:
		return "internal error"
	case .Timeout:
		return "timeout"
	case .Truncated:
		return "truncated"
	}
	return "unknown error"
}
