// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");

pub fn version() []const u8 {
    const v = c.rns_version() orelse return "";
    return std.mem.span(v);
}

pub fn lastError(allocator: std.mem.Allocator) types.Error![]u8 {
    var buf: [512]u8 = undefined;
    var written: usize = 0;
    try types.mapCode(c.rns_last_error(&buf, buf.len, &written));
    const n = @min(written, buf.len);
    return allocator.dupe(u8, buf[0..n]) catch return error.Internal;
}

pub fn errorString(err: types.Error) []const u8 {
    return switch (err) {
        error.InvalidArg => "invalid argument",
        error.InvalidHandle => "invalid handle",
        error.NotFound => "not found",
        error.State => "invalid state",
        error.Io => "io error",
        error.Internal => "internal error",
        error.Timeout => "timeout",
        error.Truncated => "truncated",
    };
}
