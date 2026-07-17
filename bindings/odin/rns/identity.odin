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
