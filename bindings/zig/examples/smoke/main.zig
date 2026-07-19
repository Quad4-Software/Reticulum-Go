// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");

pub fn main() !void {
    if (!std.mem.eql(u8, rns.version(), rns.api_version)) {
        std.debug.print("unexpected version: {s}\n", .{rns.version()});
        std.process.exit(1);
    }

    const node = try rns.nodeCreate("");
    defer rns.nodeDestroy(node) catch {};

    try rns.nodeStart(node);
    defer rns.nodeStop(node) catch {};

    const err = rns.eventPoll(node, 10, &.{});
    if (err != error.Timeout) {
        std.debug.print("expected timeout poll on idle node\n", .{});
        std.process.exit(1);
    }

    std.debug.print("zig-smoke ok\n", .{});
}
