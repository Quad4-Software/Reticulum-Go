// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

link_open :: proc(node: Node, dest_hash: []u8) -> (link: Link, err: Error) {
	if len(dest_hash) != HASH_LEN {
		return 0, .Invalid_Arg
	}
	h := rns_link_open(u64(node), raw_data(dest_hash))
	if h == 0 {
		return 0, .Internal
	}
	return Link(h), .Ok
}

link_send :: proc(link: Link, data: []u8) -> Error {
	if len(data) == 0 {
		return .Invalid_Arg
	}
	return Error(rns_link_send(u64(link), raw_data(data), c.size_t(len(data))))
}

link_send_resource :: proc(link: Link, data: []u8, name: string = "") -> Error {
	name_cstr: cstring
	if name != "" {
		name_cstr = strings.clone_to_cstring(name)
		defer delete(name_cstr)
	}
	data_ptr: [^]u8
	data_len: c.size_t
	if len(data) > 0 {
		data_ptr = raw_data(data)
		data_len = c.size_t(len(data))
	}
	return Error(rns_link_send_resource(u64(link), data_ptr, data_len, name_cstr))
}

link_close :: proc(link: Link) -> Error {
	return Error(rns_link_close(u64(link)))
}

link_id :: proc(link: Link) -> (id: [HASH_LEN]u8, err: Error) {
	written: c.size_t
	code := Error(rns_link_id(u64(link), raw_data(id[:]), HASH_LEN, &written))
	if code != .Ok {
		return {}, code
	}
	if written != HASH_LEN {
		return {}, .Truncated
	}
	return id, .Ok
}

link_request :: proc(
	node: Node,
	link: Link,
	path: string,
	data: []u8 = nil,
	timeout_ms: i32 = 5000,
) -> (request_id: [HASH_LEN]u8, err: Error) {
	if path == "" {
		return {}, .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	written: c.size_t
	data_ptr: [^]u8
	data_len: c.size_t
	if len(data) > 0 {
		data_ptr = raw_data(data)
		data_len = c.size_t(len(data))
	}
	code := Error(rns_link_request(
		u64(node),
		u64(link),
		path_c,
		data_ptr,
		data_len,
		c.int(timeout_ms),
		raw_data(request_id[:]),
		HASH_LEN,
		&written,
	))
	if code != .Ok {
		return {}, code
	}
	if written != HASH_LEN {
		return {}, .Truncated
	}
	return request_id, .Ok
}

request_respond :: proc(node: Node, request_id: []u8, data: []u8 = nil) -> Error {
	if len(request_id) == 0 {
		return .Invalid_Arg
	}
	data_ptr: [^]u8
	data_len: c.size_t
	if len(data) > 0 {
		data_ptr = raw_data(data)
		data_len = c.size_t(len(data))
	}
	return Error(rns_request_respond(
		u64(node),
		raw_data(request_id),
		c.size_t(len(request_id)),
		data_ptr,
		data_len,
	))
}

request_respond_file :: proc(node: Node, request_id: []u8, filename: string, data: []u8 = nil) -> Error {
	if len(request_id) == 0 || filename == "" {
		return .Invalid_Arg
	}
	filename_cstr := strings.clone_to_cstring(filename)
	defer delete(filename_cstr)
	data_ptr: [^]u8
	data_len: c.size_t
	if len(data) > 0 {
		data_ptr = raw_data(data)
		data_len = c.size_t(len(data))
	}
	return Error(rns_request_respond_file(
		u64(node),
		raw_data(request_id),
		c.size_t(len(request_id)),
		filename_cstr,
		data_ptr,
		data_len,
	))
}
