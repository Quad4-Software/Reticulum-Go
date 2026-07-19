// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

/*
RSG and RSM helpers wrap librns signature create and verify.
*/

rsg_create :: proc(
	identity: Identity,
	message: []u8,
	embed: bool,
	allocator := context.allocator,
) -> (
	blob: []u8,
	err: Error,
) {
	needed: c.size_t
	msg_ptr: [^]u8 = nil
	if len(message) > 0 {
		msg_ptr = raw_data(message)
	}
	code := Error(
		rns_rsg_create(
			u64(identity),
			msg_ptr,
			c.size_t(len(message)),
			c.int(embed ? 1 : 0),
			nil,
			0,
			&needed,
		),
	)
	if code != .Truncated && code != .Ok {
		return nil, code
	}
	if needed == 0 {
		return nil, .Internal
	}
	out := make([]u8, int(needed), allocator)
	written: c.size_t
	code = Error(
		rns_rsg_create(
			u64(identity),
			msg_ptr,
			c.size_t(len(message)),
			c.int(embed ? 1 : 0),
			raw_data(out),
			c.size_t(len(out)),
			&written,
		),
	)
	if code != .Ok {
		delete(out)
		return nil, code
	}
	return out[:int(written)], .Ok
}

rsg_validate :: proc(rsg, message, required_signer_hash: []u8) -> Error {
	rsg_ptr: [^]u8 = nil
	msg_ptr: [^]u8 = nil
	req_ptr: [^]u8 = nil
	if len(rsg) > 0 {
		rsg_ptr = raw_data(rsg)
	}
	if len(message) > 0 {
		msg_ptr = raw_data(message)
	}
	if len(required_signer_hash) > 0 {
		req_ptr = raw_data(required_signer_hash)
	}
	return Error(
		rns_rsg_validate(
			rsg_ptr,
			c.size_t(len(rsg)),
			msg_ptr,
			c.size_t(len(message)),
			req_ptr,
			c.size_t(len(required_signer_hash)),
		),
	)
}

rsg_sign_file :: proc(identity: Identity, path: string, allocator := context.allocator) -> (blob: []u8, err: Error) {
	if path == "" {
		return nil, .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	needed: c.size_t
	code := Error(rns_rsg_sign_file(u64(identity), path_c, nil, 0, &needed))
	if code != .Truncated && code != .Ok {
		return nil, code
	}
	if needed == 0 {
		return nil, .Internal
	}
	out := make([]u8, int(needed), allocator)
	written: c.size_t
	code = Error(rns_rsg_sign_file(u64(identity), path_c, raw_data(out), c.size_t(len(out)), &written))
	if code != .Ok {
		delete(out)
		return nil, code
	}
	return out[:int(written)], .Ok
}

rsg_verify_file :: proc(rsg: []u8, path: string, required_signer_hash: []u8) -> Error {
	if path == "" {
		return .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	rsg_ptr: [^]u8 = nil
	req_ptr: [^]u8 = nil
	if len(rsg) > 0 {
		rsg_ptr = raw_data(rsg)
	}
	if len(required_signer_hash) > 0 {
		req_ptr = raw_data(required_signer_hash)
	}
	return Error(
		rns_rsg_verify_file(
			rsg_ptr,
			c.size_t(len(rsg)),
			path_c,
			req_ptr,
			c.size_t(len(required_signer_hash)),
		),
	)
}

rsm_verify :: proc(
	rsm, required_signer_hash: []u8,
	allocator := context.allocator,
) -> (
	message: []u8,
	err: Error,
) {
	rsm_ptr: [^]u8 = nil
	req_ptr: [^]u8 = nil
	if len(rsm) > 0 {
		rsm_ptr = raw_data(rsm)
	}
	if len(required_signer_hash) > 0 {
		req_ptr = raw_data(required_signer_hash)
	}
	needed: c.size_t
	code := Error(
		rns_rsm_verify(
			rsm_ptr,
			c.size_t(len(rsm)),
			req_ptr,
			c.size_t(len(required_signer_hash)),
			nil,
			0,
			&needed,
		),
	)
	if code != .Truncated && code != .Ok {
		return nil, code
	}
	if needed == 0 {
		return nil, .Ok
	}
	out := make([]u8, int(needed), allocator)
	written: c.size_t
	code = Error(
		rns_rsm_verify(
			rsm_ptr,
			c.size_t(len(rsm)),
			req_ptr,
			c.size_t(len(required_signer_hash)),
			raw_data(out),
			c.size_t(len(out)),
			&written,
		),
	)
	if code != .Ok {
		delete(out)
		return nil, code
	}
	return out[:int(written)], .Ok
}
