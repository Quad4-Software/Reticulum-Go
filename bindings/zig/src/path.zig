// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const c = @import("c.zig");
const types = @import("types.zig");

pub fn request(node: types.Node, dest_hash: []const u8) types.Error!void {
    if (dest_hash.len != types.hash_len) return error.InvalidArg;
    try types.mapCode(c.rns_path_request(types.asU64(node), dest_hash.ptr));
}

pub fn table(node: types.Node, out: []types.PathEntry, max_hops: c_int) types.Error!usize {
    if (out.len == 0) return error.InvalidArg;
    var written: usize = 0;
    try types.mapCode(c.rns_path_table(types.asU64(node), out.ptr, out.len, &written, max_hops));
    return written;
}
