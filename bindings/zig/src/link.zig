// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn open(node: types.Node, dest_hash: []const u8) types.Error!types.Link {
    if (dest_hash.len != types.hash_len) return error.InvalidArg;
    const h = c.rns_link_open(types.asU64(node), dest_hash.ptr);
    if (h == 0) return error.Internal;
    return @enumFromInt(h);
}

pub fn send(link: types.Link, data: []const u8) types.Error!void {
    if (data.len == 0) return error.InvalidArg;
    try types.mapCode(c.rns_link_send(types.asU64(link), data.ptr, data.len));
}

pub fn sendResource(link: types.Link, data: []const u8, name: []const u8) types.Error!void {
    var name_buf: [256]u8 = undefined;
    const name_z = if (name.len == 0) null else try util.requireCString(name, &name_buf);
    try types.mapCode(c.rns_link_send_resource(
        types.asU64(link),
        if (data.len == 0) null else data.ptr,
        data.len,
        name_z,
    ));
}

pub fn close(link: types.Link) types.Error!void {
    try types.mapCode(c.rns_link_close(types.asU64(link)));
}

pub fn id(link: types.Link) types.Error![types.hash_len]u8 {
    var out: [types.hash_len]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_link_id(types.asU64(link), &out, out.len, &written));
    if (written != types.hash_len) return error.Truncated;
    return out;
}

pub fn request(
    node: types.Node,
    link: types.Link,
    path: []const u8,
    data: []const u8,
    timeout_ms: c_int,
) types.Error![types.hash_len]u8 {
    var path_buf: [256]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    var request_id: [types.hash_len]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_link_request(
        types.asU64(node),
        types.asU64(link),
        path_z,
        if (data.len == 0) null else data.ptr,
        data.len,
        timeout_ms,
        &request_id,
        request_id.len,
        &written,
    ));
    if (written != types.hash_len) return error.Truncated;
    return request_id;
}

pub fn respond(node: types.Node, request_id: []const u8, data: []const u8) types.Error!void {
    if (request_id.len == 0) return error.InvalidArg;
    try types.mapCode(c.rns_request_respond(
        types.asU64(node),
        request_id.ptr,
        request_id.len,
        if (data.len == 0) null else data.ptr,
        data.len,
    ));
}

pub fn respondFile(node: types.Node, request_id: []const u8, filename: []const u8, data: []const u8) types.Error!void {
    if (request_id.len == 0 or filename.len == 0) return error.InvalidArg;
    var name_buf: [256]u8 = undefined;
    const name_z = try util.requireCString(filename, &name_buf);
    try types.mapCode(c.rns_request_respond_file(
        types.asU64(node),
        request_id.ptr,
        request_id.len,
        name_z,
        if (data.len == 0) null else data.ptr,
        data.len,
    ));
}
