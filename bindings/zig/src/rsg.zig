// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

fn mapSizeProbe(code: c_int) types.Error!void {
    if (code == c.RNS_OK or code == c.RNS_ERR_TRUNCATED) return;
    try types.mapCode(code);
}

pub fn create(identity: types.Identity, message: []const u8, embed: bool, allocator: std.mem.Allocator) types.Error![]u8 {
    var needed: usize = 0;
    try mapSizeProbe(c.rns_rsg_create(
        types.asU64(identity),
        if (message.len == 0) null else message.ptr,
        message.len,
        if (embed) 1 else 0,
        null,
        0,
        &needed,
    ));
    if (needed == 0) return error.Internal;
    const out = allocator.alloc(u8, needed) catch return error.Internal;
    errdefer allocator.free(out);
    var written: usize = 0;
    try types.mapCode(c.rns_rsg_create(
        types.asU64(identity),
        if (message.len == 0) null else message.ptr,
        message.len,
        if (embed) 1 else 0,
        out.ptr,
        out.len,
        &written,
    ));
    if (written == out.len) return out;
    const trimmed = allocator.dupe(u8, out[0..written]) catch {
        allocator.free(out);
        return error.Internal;
    };
    allocator.free(out);
    return trimmed;
}

pub fn validate(rsg: []const u8, message: []const u8, required_signer_hash: []const u8) types.Error!void {
    try types.mapCode(c.rns_rsg_validate(
        if (rsg.len == 0) null else rsg.ptr,
        rsg.len,
        if (message.len == 0) null else message.ptr,
        message.len,
        if (required_signer_hash.len == 0) null else required_signer_hash.ptr,
        required_signer_hash.len,
    ));
}

pub fn signFile(identity: types.Identity, path: []const u8, allocator: std.mem.Allocator) types.Error![]u8 {
    var path_buf: [std.fs.max_path_bytes + 1]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    var needed: usize = 0;
    try mapSizeProbe(c.rns_rsg_sign_file(types.asU64(identity), path_z, null, 0, &needed));
    if (needed == 0) return error.Internal;
    const out = allocator.alloc(u8, needed) catch return error.Internal;
    errdefer allocator.free(out);
    var written: usize = 0;
    try types.mapCode(c.rns_rsg_sign_file(types.asU64(identity), path_z, out.ptr, out.len, &written));
    if (written == out.len) return out;
    const trimmed = allocator.dupe(u8, out[0..written]) catch {
        allocator.free(out);
        return error.Internal;
    };
    allocator.free(out);
    return trimmed;
}

pub fn verifyFile(rsg: []const u8, path: []const u8, required_signer_hash: []const u8) types.Error!void {
    var path_buf: [std.fs.max_path_bytes + 1]u8 = undefined;
    const path_z = try util.requireCString(path, &path_buf);
    try types.mapCode(c.rns_rsg_verify_file(
        if (rsg.len == 0) null else rsg.ptr,
        rsg.len,
        path_z,
        if (required_signer_hash.len == 0) null else required_signer_hash.ptr,
        required_signer_hash.len,
    ));
}

pub fn rsmVerify(rsm: []const u8, required_signer_hash: []const u8, allocator: std.mem.Allocator) types.Error![]u8 {
    var needed: usize = 0;
    try mapSizeProbe(c.rns_rsm_verify(
        if (rsm.len == 0) null else rsm.ptr,
        rsm.len,
        if (required_signer_hash.len == 0) null else required_signer_hash.ptr,
        required_signer_hash.len,
        null,
        0,
        &needed,
    ));
    if (needed == 0) return &.{};
    const out = allocator.alloc(u8, needed) catch return error.Internal;
    errdefer allocator.free(out);
    var written: usize = 0;
    try types.mapCode(c.rns_rsm_verify(
        if (rsm.len == 0) null else rsm.ptr,
        rsm.len,
        if (required_signer_hash.len == 0) null else required_signer_hash.ptr,
        required_signer_hash.len,
        out.ptr,
        out.len,
        &written,
    ));
    if (written == out.len) return out;
    const trimmed = allocator.dupe(u8, out[0..written]) catch {
        allocator.free(out);
        return error.Internal;
    };
    allocator.free(out);
    return trimmed;
}
