// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");
const testing = std.testing;
const helpers = @import("helpers.zig");

test "link announce open send over udp" {
    const port_a = try helpers.freeUdpPort();
    const port_b = try helpers.freeUdpPort();

    var tmp_a = testing.tmpDir(.{});
    defer tmp_a.cleanup();
    var tmp_b = testing.tmpDir(.{});
    defer tmp_b.cleanup();

    try helpers.writeUdpPeerConfig(tmp_a.dir, port_a, port_b);
    try helpers.writeUdpPeerConfig(tmp_b.dir, port_b, port_a);

    const cfg_a = try helpers.configPath(tmp_a, testing.allocator);
    defer testing.allocator.free(cfg_a);
    const cfg_b = try helpers.configPath(tmp_b, testing.allocator);
    defer testing.allocator.free(cfg_b);

    const node_a = try rns.nodeCreate(cfg_a);
    defer rns.nodeDestroy(node_a) catch {};
    const node_b = try rns.nodeCreate(cfg_b);
    defer rns.nodeDestroy(node_b) catch {};

    const id_a = try rns.identityGenerate();
    defer rns.identityDestroy(id_a) catch {};
    const id_b = try rns.identityGenerate();
    defer rns.identityDestroy(id_b) catch {};

    try rns.nodeSetIdentity(node_a, id_a);
    try rns.nodeSetIdentity(node_b, id_b);
    try rns.nodeStart(node_a);
    defer rns.nodeStop(node_a) catch {};
    try rns.nodeStart(node_b);
    defer rns.nodeStop(node_b) catch {};

    const dest_a = try rns.destinationCreate(node_a, @enumFromInt(0), "zig-rns", &.{"link"}, true);
    defer rns.destinationDestroy(dest_a) catch {};

    const app_payload = "zig-app";
    try rns.destinationAnnounce(dest_a, app_payload);

    var app_buf: [256]u8 = undefined;
    const announce = try helpers.pollUntil(node_b, .announce, 5000, &app_buf);
    try testing.expectEqualSlices(u8, app_payload, rns.eventAppData(&announce));

    const dest_hash = try rns.destinationHash(dest_a);
    try testing.expectEqualSlices(u8, &dest_hash, rns.eventDestinationHash(&announce));

    const link_b = try rns.linkOpen(node_b, &dest_hash);
    defer rns.linkClose(link_b) catch {};

    _ = try helpers.pollUntil(node_b, .link_established, 5000, &.{});
    _ = try helpers.pollUntil(node_a, .link_established, 5000, &.{});

    const payload = "zig-link-payload";
    try rns.linkSend(link_b, payload);

    var data_buf: [256]u8 = undefined;
    const data_ev = try helpers.pollUntil(node_a, .link_data, 5000, &data_buf);
    try testing.expectEqualSlices(u8, payload, rns.eventAppData(&data_ev));
}
