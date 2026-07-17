// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");
const testing = std.testing;

test "destination create announce hash" {
    const node = try rns.nodeCreate("");
    defer rns.nodeDestroy(node) catch {};

    const id = try rns.identityGenerate();
    defer rns.identityDestroy(id) catch {};

    try rns.nodeSetIdentity(node, id);
    try rns.nodeStart(node);
    defer rns.nodeStop(node) catch {};

    const dest = try rns.destinationCreate(node, @enumFromInt(0), "zig-rns", &.{"chat"}, true);
    defer rns.destinationDestroy(dest) catch {};

    try rns.destinationAnnounce(dest, "hello");

    const hash = try rns.destinationHash(dest);
    try testing.expectEqual(@as(usize, rns.hash_len), hash.len);

    var entries: [8]rns.PathEntry = undefined;
    _ = rns.pathTable(node, &entries, -1) catch |err| switch (err) {
        error.NotFound => {},
        else => return err,
    };
}
