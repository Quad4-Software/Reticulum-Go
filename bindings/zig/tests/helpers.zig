// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const rns = @import("rns");
const testing = std.testing;
const Io = std.Io;

pub fn freeUdpPort() !u16 {
    var addr: Io.net.IpAddress = .{ .ip4 = .loopback(0) };
    const sock = try addr.bind(testing.io, .{ .mode = .dgram, .protocol = .udp });
    defer sock.close(testing.io);
    return sock.address.getPort();
}

pub fn writeUdpPeerConfig(dir: Io.Dir, listen: u16, peer: u16) !void {
    var buf: [512]u8 = undefined;
    const cfg = try std.fmt.bufPrint(&buf,
        \\[reticulum]
        \\enable_transport = yes
        \\share_instance = no
        \\
        \\[interfaces]
        \\  [[UDP]]
        \\    type = UDPInterface
        \\    enabled = yes
        \\    listen_ip = 127.0.0.1
        \\    listen_port = {d}
        \\    target_host = 127.0.0.1
        \\    target_port = {d}
        \\
    , .{ listen, peer });
    try dir.writeFile(testing.io, .{ .sub_path = "config", .data = cfg });
}

pub fn configPath(tmp: testing.TmpDir, allocator: std.mem.Allocator) ![]u8 {
    return try std.fmt.allocPrint(allocator, ".zig-cache/tmp/{s}/config", .{tmp.sub_path});
}

pub fn pollUntil(node: rns.Node, want: rns.EventKind, timeout_ms: i64, app_buf: []u8) !rns.Event {
    var remaining = timeout_ms;
    while (remaining > 0) {
        const step: c_int = 50;
        if (rns.eventPoll(node, step, app_buf)) |ev| {
            if (@as(rns.EventKind, @enumFromInt(ev.kind)) == want) return ev;
        } else |err| switch (err) {
            error.Timeout => {},
            else => return err,
        }
        remaining -= step;
    }
    return error.Timeout;
}
