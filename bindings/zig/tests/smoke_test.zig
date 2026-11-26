// SPDX-License-Identifier: LicenseRef-Reticulum
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


test "added abi surface smoke" {
    const node = try rns.nodeCreate("");
    defer rns.nodeDestroy(node) catch {};
    try testing.expectError(error.InvalidArg, rns.nodeReloadConfig(node));
    const configured = try rns.nodeCreate("tests/reload.conf");
    try rns.nodeReloadConfig(configured);
    try rns.nodeDestroy(configured);
    const unknown = std.mem.zeroes([16]u8);
    try testing.expectError(error.NotFound, rns.destinationEncrypt(&unknown, "x", testing.allocator));
    try testing.expectError(error.State, rns.packetSend(node, &unknown, "x"));
    try testing.expectError(error.NotFound, rns.linkFromId(node, &unknown));

    const identity = try rns.identityGenerate();
    defer rns.identityDestroy(identity) catch {};
    const dest = try rns.destinationCreate(node, identity, "zigtest", &.{}, false);
    defer rns.destinationDestroy(dest) catch {};
    try rns.destinationSetProofStrategy(dest, 0);
    try testing.expectError(error.InvalidArg, rns.destinationSetProofStrategy(dest, 255));
}
