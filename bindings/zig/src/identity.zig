// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn generate() types.Error!types.Identity {
    const h = c.rns_identity_generate();
    if (h == 0) return error.Internal;
    return @enumFromInt(h);
}

pub fn load(path: []const u8) types.Error!types.Identity {
    var path_buf: [std.fs.max_path_bytes + 1]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    const h = c.rns_identity_load(path_z);
    if (h == 0) return error.Io;
    return @enumFromInt(h);
}

pub fn save(identity: types.Identity, path: []const u8) types.Error!void {
    var path_buf: [std.fs.max_path_bytes + 1]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    try types.mapCode(c.rns_identity_save(types.asU64(identity), path_z));
}

pub fn destroy(identity: types.Identity) types.Error!void {
    try types.mapCode(c.rns_identity_destroy(types.asU64(identity)));
}

pub fn hash(identity: types.Identity, allocator: std.mem.Allocator) types.Error![]u8 {
    var buf: [64]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_identity_hash(types.asU64(identity), &buf, buf.len, &written));
    const n = @min(written, buf.len);
    return allocator.dupe(u8, buf[0..n]) catch return error.Internal;
}
