// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"
import "core:strings"

identity_generate :: proc() -> (identity: Identity, err: Error) {
	h := rns_identity_generate()
	if h == 0 {
		return 0, .Internal
	}
	return Identity(h), .Ok
}

identity_load :: proc(path: string) -> (identity: Identity, err: Error) {
	if path == "" {
		return 0, .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	h := rns_identity_load(path_c)
	if h == 0 {
		return 0, .IO
	}
	return Identity(h), .Ok
}

identity_save :: proc(identity: Identity, path: string) -> Error {
	if path == "" {
		return .Invalid_Arg
	}
	path_c := strings.clone_to_cstring(path, context.temp_allocator)
	return Error(rns_identity_save(u64(identity), path_c))
}

identity_destroy :: proc(identity: Identity) -> Error {
	return Error(rns_identity_destroy(u64(identity)))
}

identity_hash :: proc(identity: Identity, allocator := context.allocator) -> (hex: string, err: Error) {
	buf: [64]u8
	written: c.size_t
	code := Error(rns_identity_hash(u64(identity), raw_data(buf[:]), len(buf), &written))
	if code != .Ok {
		return "", code
	}
	n := min(int(written), len(buf))
	return strings.clone(string(buf[:n]), allocator), .Ok
}

identity_hash_bytes :: proc(identity: Identity, allocator := context.allocator) -> (hash: []u8, err: Error) {
	buf: [HASH_LEN]u8
	written: c.size_t
	code := Error(rns_identity_hash_bytes(u64(identity), raw_data(buf[:]), len(buf), &written))
	if code != .Ok {
		return nil, code
	}
	n := min(int(written), len(buf))
	out := make([]u8, n, allocator)
	copy(out, buf[:n])
	return out, .Ok
}

identity_public_key :: proc(identity: Identity, allocator := context.allocator) -> (pub: []u8, err: Error) {
	buf: [64]u8
	written: c.size_t
	code := Error(rns_identity_public_key(u64(identity), raw_data(buf[:]), len(buf), &written))
	if code != .Ok {
		return nil, code
	}
	n := min(int(written), len(buf))
	out := make([]u8, n, allocator)
	copy(out, buf[:n])
	return out, .Ok
}

identity_from_public_key :: proc(pub: []u8) -> (identity: Identity, err: Error) {
	if len(pub) == 0 {
		return 0, .Invalid_Arg
	}
	h := rns_identity_from_public_key(raw_data(pub), c.size_t(len(pub)))
	if h == 0 {
		return 0, .Invalid_Arg
	}
	return Identity(h), .Ok
}

identity_sign :: proc(identity: Identity, data: []u8, allocator := context.allocator) -> (sig: []u8, err: Error) {
	buf: [64]u8
	written: c.size_t
	data_ptr: [^]u8 = nil
	if len(data) > 0 {
		data_ptr = raw_data(data)
	}
	code := Error(rns_identity_sign(u64(identity), data_ptr, c.size_t(len(data)), raw_data(buf[:]), len(buf), &written))
	if code != .Ok {
		return nil, code
	}
	n := min(int(written), len(buf))
	out := make([]u8, n, allocator)
	copy(out, buf[:n])
	return out, .Ok
}

identity_verify :: proc(identity: Identity, data, signature: []u8) -> Error {
	data_ptr: [^]u8 = nil
	sig_ptr: [^]u8 = nil
	if len(data) > 0 {
		data_ptr = raw_data(data)
	}
	if len(signature) > 0 {
		sig_ptr = raw_data(signature)
	}
	return Error(rns_identity_verify(u64(identity), data_ptr, c.size_t(len(data)), sig_ptr, c.size_t(len(signature))))
}
