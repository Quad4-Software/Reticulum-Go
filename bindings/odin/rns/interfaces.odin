// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"

interfaces_list :: proc(
	node: Node,
	out: []Interface_Entry,
) -> (written: int, err: Error) {
	if len(out) == 0 {
		return 0, .Invalid_Arg
	}
	n: c.size_t
	code := Error(rns_interfaces(
		u64(node),
		raw_data(out),
		c.size_t(len(out)),
		&n,
	))
	if code != .Ok && code != .Truncated {
		return 0, code
	}
	return int(n), code
}

interface_name :: proc(e: ^Interface_Entry) -> string {
	return cstring_field(e.name[:])
}

interface_type :: proc(e: ^Interface_Entry) -> string {
	return cstring_field(e.type_name[:])
}
