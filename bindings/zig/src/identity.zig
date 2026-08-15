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

pub fn hashBytes(identity: types.Identity) types.Error![types.hash_len]u8 {
    var out: [types.hash_len]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_identity_hash_bytes(types.asU64(identity), &out, out.len, &written));
    if (written != types.hash_len) return error.Truncated;
    return out;
}

pub fn publicKey(identity: types.Identity, allocator: std.mem.Allocator) types.Error![]u8 {
    var buf: [64]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_identity_public_key(types.asU64(identity), &buf, buf.len, &written));
    const n = @min(written, buf.len);
    return allocator.dupe(u8, buf[0..n]) catch return error.Internal;
}

pub fn fromPublicKey(pub_key: []const u8) types.Error!types.Identity {
    if (pub_key.len == 0) return error.InvalidArg;
    const h = c.rns_identity_from_public_key(pub_key.ptr, pub_key.len);
    if (h == 0) return error.InvalidArg;
    return @enumFromInt(h);
}

pub fn sign(identity: types.Identity, data: []const u8, allocator: std.mem.Allocator) types.Error![]u8 {
    var buf: [64]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_identity_sign(
        types.asU64(identity),
        if (data.len == 0) null else data.ptr,
        data.len,
        &buf,
        buf.len,
        &written,
    ));
    const n = @min(written, buf.len);
    return allocator.dupe(u8, buf[0..n]) catch return error.Internal;
}

pub fn verify(identity: types.Identity, data: []const u8, signature: []const u8) types.Error!void {
    try types.mapCode(c.rns_identity_verify(
        types.asU64(identity),
        if (data.len == 0) null else data.ptr,
        data.len,
        if (signature.len == 0) null else signature.ptr,
        signature.len,
    ));
}
