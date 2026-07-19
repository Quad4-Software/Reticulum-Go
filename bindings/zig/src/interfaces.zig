// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn list(node: types.Node, out: []types.InterfaceEntry) types.Error!usize {
    if (out.len == 0) return error.InvalidArg;
    var written: usize = 0;
    const code = c.rns_interfaces(types.asU64(node), out.ptr, out.len, &written);
    if (code != c.RNS_OK and code != c.RNS_ERR_TRUNCATED) {
        try types.mapCode(code);
    }
    return written;
}

pub fn name(entry: *const types.InterfaceEntry) []const u8 {
    return util.cstringField(&entry.name);
}

pub fn typeName(entry: *const types.InterfaceEntry) []const u8 {
    return util.cstringField(&entry.type_name);
}
