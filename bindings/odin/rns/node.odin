// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

node_create :: proc(config_path: string = "") -> (node: Node, err: Error) {
	path_c: cstring
	if config_path != "" {
		path_c = strings.clone_to_cstring(config_path, context.temp_allocator)
	} else {
		path_c = ""
	}
	h := rns_node_create(path_c)
	if h == 0 {
		return 0, .Internal
	}
	return Node(h), .Ok
}

node_start :: proc(node: Node) -> Error {
	return Error(rns_node_start(u64(node)))
}

node_stop :: proc(node: Node) -> Error {
	return Error(rns_node_stop(u64(node)))
}

node_destroy :: proc(node: Node) -> Error {
	return Error(rns_node_destroy(u64(node)))
}

node_set_identity :: proc(node: Node, identity: Identity) -> Error {
	return Error(rns_node_set_identity(u64(node), u64(identity)))
}

node_pause :: proc(node: Node) -> Error {
	return Error(rns_node_pause(u64(node)))
}

node_resume :: proc(node: Node) -> Error {
	return Error(rns_node_resume(u64(node)))
}

node_refresh_paths :: proc(node: Node, dest_hashes: [][HASH_LEN]u8 = nil) -> Error {
	if len(dest_hashes) == 0 {
		return Error(rns_node_refresh_paths(u64(node), nil, 0))
	}
	flat := make([]u8, len(dest_hashes) * HASH_LEN, context.temp_allocator)
	for h, i in dest_hashes {
		hh := h
		copy(flat[i * HASH_LEN:(i + 1) * HASH_LEN], hh[:])
	}
	return Error(rns_node_refresh_paths(u64(node), raw_data(flat), c.size_t(len(dest_hashes))))
}
