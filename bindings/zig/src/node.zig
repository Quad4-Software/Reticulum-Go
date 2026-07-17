// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn create(config_path: []const u8) types.Error!types.Node {
    var path_buf: [std.fs.max_path_bytes + 1]u8 = undefined;
    const path_z = try util.toCString(config_path, &path_buf);
    const h = c.rns_node_create(path_z);
    if (h == 0) return error.Internal;
    return @enumFromInt(h);
}

pub fn start(node: types.Node) types.Error!void {
    try types.mapCode(c.rns_node_start(types.asU64(node)));
}

pub fn stop(node: types.Node) types.Error!void {
    try types.mapCode(c.rns_node_stop(types.asU64(node)));
}

pub fn destroy(node: types.Node) types.Error!void {
    try types.mapCode(c.rns_node_destroy(types.asU64(node)));
}

pub fn setIdentity(node: types.Node, identity: types.Identity) types.Error!void {
    try types.mapCode(c.rns_node_set_identity(types.asU64(node), types.asU64(identity)));
}

pub fn pause(node: types.Node) types.Error!void {
    try types.mapCode(c.rns_node_pause(types.asU64(node)));
}

pub fn resumeNode(node: types.Node) types.Error!void {
    try types.mapCode(c.rns_node_resume(types.asU64(node)));
}

pub fn refreshPaths(node: types.Node, dest_hashes: []const [types.hash_len]u8) types.Error!void {
    if (dest_hashes.len == 0) {
        try types.mapCode(c.rns_node_refresh_paths(types.asU64(node), null, 0));
        return;
    }
    var flat_buf: [16 * types.hash_len]u8 = undefined;
    if (dest_hashes.len * types.hash_len > flat_buf.len) return error.InvalidArg;
    for (dest_hashes, 0..) |h, i| {
        @memcpy(flat_buf[i * types.hash_len ..][0..types.hash_len], &h);
    }
    try types.mapCode(c.rns_node_refresh_paths(
        types.asU64(node),
        flat_buf[0 .. dest_hashes.len * types.hash_len].ptr,
        dest_hashes.len,
    ));
}
