// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

destination_create :: proc(
	node: Node,
	identity: Identity = 0,
	app_name: string,
	aspects: []string = nil,
	accepts_links: bool = true,
) -> (destination: Destination, err: Error) {
	if app_name == "" {
		return 0, .Invalid_Arg
	}
	app_c := strings.clone_to_cstring(app_name, context.temp_allocator)
	aspect_ptrs: [^]cstring
	aspect_count: c.size_t
	if len(aspects) > 0 {
		arr := make([]cstring, len(aspects), context.temp_allocator)
		for a, i in aspects {
			arr[i] = strings.clone_to_cstring(a, context.temp_allocator)
		}
		aspect_ptrs = raw_data(arr)
		aspect_count = c.size_t(len(aspects))
	}
	h := rns_destination_create(
		u64(node),
		u64(identity),
		app_c,
		aspect_ptrs,
		aspect_count,
		1 if accepts_links else 0,
	)
	if h == 0 {
		return 0, .Internal
	}
	return Destination(h), .Ok
}

destination_announce :: proc(destination: Destination, app_data: []u8 = nil) -> Error {
	if len(app_data) == 0 {
		return Error(rns_destination_announce(u64(destination), nil, 0))
	}
	return Error(rns_destination_announce(u64(destination), raw_data(app_data), c.size_t(len(app_data))))
}

destination_hash :: proc(destination: Destination) -> (hash: [HASH_LEN]u8, err: Error) {
	written: c.size_t
	code := Error(rns_destination_hash(u64(destination), raw_data(hash[:]), HASH_LEN, &written))
	if code != .Ok {
		return {}, code
	}
	if written != HASH_LEN {
		return {}, .Truncated
	}
	return hash, .Ok
}

destination_destroy :: proc(destination: Destination) -> Error {
	return Error(rns_destination_destroy(u64(destination)))
}

destination_register_request_handler :: proc(destination: Destination, path: string) -> Error {
	if path == "" {
		return .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	return Error(rns_destination_register_request_handler(u64(destination), path_c))
}
