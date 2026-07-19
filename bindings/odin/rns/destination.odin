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

// Encrypt plaintext for a recalled destination hash (LXMF prop / opportunistic).
destination_encrypt :: proc(dest_hash: []u8, plaintext: []u8, allocator := context.allocator) -> (out: []u8, err: Error) {
	if len(dest_hash) != HASH_LEN {
		return nil, .Invalid_Arg
	}
	cap := len(plaintext) + 256
	buf := make([]u8, cap, context.temp_allocator)
	written: c.size_t
	code := Error(rns_destination_encrypt(
		raw_data(dest_hash),
		raw_data(plaintext) if len(plaintext) > 0 else nil,
		c.size_t(len(plaintext)),
		raw_data(buf),
		c.size_t(len(buf)),
		&written,
	))
	if code == .Truncated {
		buf = make([]u8, int(written) + 64, context.temp_allocator)
		written = 0
		code = Error(rns_destination_encrypt(
			raw_data(dest_hash),
			raw_data(plaintext) if len(plaintext) > 0 else nil,
			c.size_t(len(plaintext)),
			raw_data(buf),
			c.size_t(len(buf)),
			&written,
		))
	}
	if code != .Ok {
		return nil, code
	}
	out = make([]u8, int(written), allocator)
	copy(out, buf[:written])
	return out, .Ok
}

// Send an encrypted DATA packet to dest_hash (opportunistic LXMF).
packet_send :: proc(node: Node, dest_hash: []u8, plaintext: []u8) -> Error {
	if len(dest_hash) != HASH_LEN {
		return .Invalid_Arg
	}
	return Error(rns_packet_send(
		u64(node),
		raw_data(dest_hash),
		raw_data(plaintext) if len(plaintext) > 0 else nil,
		c.size_t(len(plaintext)),
	))
}
