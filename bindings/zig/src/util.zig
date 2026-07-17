// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const types = @import("types.zig");

pub fn toCString(path: []const u8, buf: []u8) types.Error!?[*:0]const u8 {
    if (path.len == 0) return null;
    if (path.len + 1 > buf.len) return error.InvalidArg;
    @memcpy(buf[0..path.len], path);
    buf[path.len] = 0;
    return buf[0..path.len :0].ptr;
}

pub fn requireCString(path: []const u8, buf: []u8) types.Error![*:0]const u8 {
    if (path.len == 0) return error.InvalidArg;
    return (try toCString(path, buf)) orelse unreachable;
}

pub fn cstringField(buf: []const u8) []const u8 {
    return std.mem.sliceTo(buf, 0);
}

pub fn hashToHex(hash: []const u8, out: []u8) types.Error![]u8 {
    if (out.len < hash.len * 2) return error.Truncated;
    const hex = "0123456789abcdef";
    for (hash, 0..) |b, i| {
        out[i * 2] = hex[b >> 4];
        out[i * 2 + 1] = hex[b & 0xf];
    }
    return out[0 .. hash.len * 2];
}

pub fn hexToHash(hex_str: []const u8, out: *[types.hash_len]u8) types.Error!void {
    if (hex_str.len != types.hash_len * 2) return error.InvalidArg;
    var i: usize = 0;
    while (i < types.hash_len) : (i += 1) {
        const hi = std.fmt.charToDigit(hex_str[i * 2], 16) catch return error.InvalidArg;
        const lo = std.fmt.charToDigit(hex_str[i * 2 + 1], 16) catch return error.InvalidArg;
        out[i] = @intCast((hi << 4) | lo);
    }
}
