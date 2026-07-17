// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");
const testing = std.testing;

test "version matches api_version" {
    try testing.expectEqualStrings(rns.api_version, rns.version());
}

test "node lifecycle" {
    const node = try rns.nodeCreate("");
    defer rns.nodeDestroy(node) catch {};

    try rns.nodeStart(node);
    defer rns.nodeStop(node) catch {};

    try testing.expectError(error.Timeout, rns.eventPoll(node, 10, &.{}));
}

test "invalid handle" {
    const bad: rns.Node = @enumFromInt(0);
    const err = rns.nodeStart(bad);
    try testing.expect(err == error.InvalidHandle or err == error.InvalidArg or err == error.Internal);
}
