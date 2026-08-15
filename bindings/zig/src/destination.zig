// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn create(
    node: types.Node,
    identity: types.Identity,
    app_name: []const u8,
    aspects: []const []const u8,
    accepts_links: bool,
) types.Error!types.Destination {
    if (app_name.len == 0) return error.InvalidArg;

    var app_buf: [256]u8 = undefined;
    const app_z = try util.requireCString(app_name, &app_buf);

    var aspect_storage: [8][128]u8 = undefined;
    var aspect_ptrs: [8][*:0]const u8 = undefined;
    if (aspects.len > aspect_ptrs.len) return error.InvalidArg;
    for (aspects, 0..) |a, i| {
        aspect_ptrs[i] = try util.requireCString(a, aspect_storage[i][0..]);
    }

    const h = c.rns_destination_create(
        types.asU64(node),
        types.asU64(identity),
        app_z,
        if (aspects.len == 0) null else aspect_ptrs[0..aspects.len].ptr,
        aspects.len,
        if (accepts_links) 1 else 0,
    );
    if (h == 0) return error.Internal;
    return @enumFromInt(h);
}

pub fn enableRatchets(destination: types.Destination, path: ?[]const u8) types.Error!void {
    if (path) |p| {
        var path_buf: [4096]u8 = undefined;
        const path_z = try util.requireCString(p, &path_buf);
        try types.mapCode(c.rns_destination_enable_ratchets(types.asU64(destination), path_z));
        return;
    }
    try types.mapCode(c.rns_destination_enable_ratchets(types.asU64(destination), null));
}

pub fn enforceRatchets(destination: types.Destination) types.Error!void {
    try types.mapCode(c.rns_destination_enforce_ratchets(types.asU64(destination)));
}

pub fn announce(destination: types.Destination, app_data: []const u8) types.Error!void {
    if (app_data.len == 0) {
        try types.mapCode(c.rns_destination_announce(types.asU64(destination), null, 0));
        return;
    }
    try types.mapCode(c.rns_destination_announce(types.asU64(destination), app_data.ptr, app_data.len));
}

pub fn hash(destination: types.Destination) types.Error![types.hash_len]u8 {
    var out: [types.hash_len]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_destination_hash(types.asU64(destination), &out, out.len, &written));
    if (written != types.hash_len) return error.Truncated;
    return out;
}

pub fn destroy(destination: types.Destination) types.Error!void {
    try types.mapCode(c.rns_destination_destroy(types.asU64(destination)));
}

pub fn registerRequestHandler(destination: types.Destination, path: []const u8) types.Error!void {
    var path_buf: [256]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    try types.mapCode(c.rns_destination_register_request_handler(types.asU64(destination), path_z));
}
