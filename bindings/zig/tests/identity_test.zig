// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");
const testing = std.testing;

test "identity generate hash destroy" {
    const id = try rns.identityGenerate();
    defer rns.identityDestroy(id) catch {};

    const hex = try rns.identityHash(id, testing.allocator);
    defer testing.allocator.free(hex);
    try testing.expectEqual(@as(usize, 32), hex.len);

    const node = try rns.nodeCreate("");
    defer rns.nodeDestroy(node) catch {};
    try rns.nodeSetIdentity(node, id);
}

test "identity load empty path" {
    try testing.expectError(error.InvalidArg, rns.identityLoad(""));
}
