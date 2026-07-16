// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"

path_request :: proc(node: Node, dest_hash: []u8) -> Error {
	if len(dest_hash) != HASH_LEN {
		return .Invalid_Arg
	}
	return Error(rns_path_request(u64(node), raw_data(dest_hash)))
}

path_table :: proc(
	node: Node,
	out: []Path_Entry,
	max_hops: i32 = -1,
) -> (written: int, err: Error) {
	if len(out) == 0 {
		return 0, .Invalid_Arg
	}
	n: c.size_t
	code := Error(rns_path_table(
		u64(node),
		raw_data(out),
		c.size_t(len(out)),
		&n,
		c.int(max_hops),
	))
	if code != .Ok {
		return 0, code
	}
	return int(n), .Ok
}
