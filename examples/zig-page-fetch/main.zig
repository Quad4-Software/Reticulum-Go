// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over the Zig librns bindings.
// Usage: zig-page-fetch [-c config] [-t timeout_sec] <dest_hash>:<page_path>

const std = @import("std");
const Io = std.Io;
const rns = @import("rns");

const page_buf_cap = 512 * 1024;
const path_table_cap = 256;
const default_timeout_sec: i64 = 60;
const path_retry_ms: i64 = 2000;

fn usage(argv0: []const u8) void {
    std.debug.print(
        \\Usage: {s} [-c config] [-t timeout_sec] <dest_hash>:<page_path>
        \\
        \\Fetch a NomadNet / pageserver page over librns (Zig bindings).
        \\
        \\Options:
        \\  -c path   Reticulum config file (required for network interfaces)
        \\  -t sec    Overall timeout in seconds (default {d})
        \\
        \\Example:
        \\  {s} -c ../librns-page-fetch/config.example 92798ea245a0afcfa559348e42d628c6:/page/index.mu
        \\
    , .{ argv0, default_timeout_sec, argv0 });
}

fn printLastError(allocator: std.mem.Allocator, what: []const u8) void {
    if (rns.lastError(allocator)) |msg| {
        defer allocator.free(msg);
        if (msg.len > 0) {
            std.debug.print("{s}: {s}\n", .{ what, msg });
            return;
        }
    } else |_| {}
    std.debug.print("{s}\n", .{what});
}

fn hashEq(a: []const u8, b: []const u8) bool {
    return a.len == rns.hash_len and b.len == rns.hash_len and std.mem.eql(u8, a, b);
}

fn pathKnown(node: rns.Node, dest: []const u8) bool {
    var table: [path_table_cap]rns.PathEntry = undefined;
    const n = rns.pathTable(node, &table, -1) catch return false;
    var i: usize = 0;
    while (i < n) : (i += 1) {
        const entry = table[i];
        if (hashEq(entry.hash[0..entry.hash_len], dest)) return true;
    }
    return false;
}

fn parseTarget(target: []const u8, hash_out: *[rns.hash_len]u8) ![]const u8 {
    const colon = std.mem.indexOfScalar(u8, target, ':') orelse return error.InvalidArg;
    if (colon == 0 or colon + 1 >= target.len) return error.InvalidArg;
    try rns.hexToHash(target[0..colon], hash_out);
    return target[colon + 1 ..];
}

fn run(allocator: std.mem.Allocator, io: Io, config_path: []const u8, target: []const u8, timeout_sec: i64) !u8 {
    const ver = rns.version();
    if (!std.mem.eql(u8, ver, rns.api_version)) {
        std.debug.print("librns version mismatch: got {s} want {s}\n", .{ ver, rns.api_version });
        return 1;
    }

    var dest_hash: [rns.hash_len]u8 = undefined;
    const page_path = parseTarget(target, &dest_hash) catch {
        std.debug.print("target must be <32-hex-dest>:<page_path>\n", .{});
        return 1;
    };

    var dest_hex_buf: [rns.hash_len * 2]u8 = undefined;
    const dest_hex = rns.hashToHex(&dest_hash, &dest_hex_buf) catch {
        std.debug.print("failed to encode destination hash\n", .{});
        return 1;
    };

    const node = rns.nodeCreate(config_path) catch {
        printLastError(allocator, "rns.nodeCreate failed");
        return 1;
    };
    defer rns.nodeDestroy(node) catch {};

    const identity = rns.identityGenerate() catch {
        printLastError(allocator, "rns.identityGenerate failed");
        return 1;
    };
    defer rns.identityDestroy(identity) catch {};

    rns.nodeSetIdentity(node, identity) catch {
        printLastError(allocator, "rns.nodeSetIdentity failed");
        return 1;
    };
    rns.nodeStart(node) catch {
        printLastError(allocator, "rns.nodeStart failed");
        return 1;
    };
    defer rns.nodeStop(node) catch {};

    std.debug.print("librns {s} fetching {s} from {s}\n", .{ ver, page_path, dest_hex });

    const page_buf = allocator.alloc(u8, page_buf_cap) catch {
        std.debug.print("out of memory\n", .{});
        return 1;
    };
    defer allocator.free(page_buf);

    const deadline_ms = timeout_sec * 1000;
    var elapsed_ms: i64 = 0;
    var last_path_req: i64 = -path_retry_ms;
    var need_path_req = true;
    var saw_announce = false;
    var link: ?rns.Link = null;

    while (elapsed_ms < deadline_ms and link == null) {
        if (need_path_req or elapsed_ms - last_path_req >= path_retry_ms) {
            rns.pathRequest(node, &dest_hash) catch {
                printLastError(allocator, "rns.pathRequest failed");
            };
            last_path_req = elapsed_ms;
            need_path_req = false;
            if (pathKnown(node, &dest_hash)) {
                std.debug.print("path known, waiting for destination identity announce\n", .{});
            } else {
                std.debug.print("requesting path to {s}\n", .{dest_hex});
            }
        }

        if (rns.eventPoll(node, 200, page_buf)) |ev| {
            const kind: rns.EventKind = @enumFromInt(ev.kind);
            if (kind == .announce and hashEq(rns.eventDestinationHash(&ev), &dest_hash)) {
                saw_announce = true;
                std.debug.print("announce for target (hops={d})\n", .{ev.hops});
                link = rns.linkOpen(node, &dest_hash) catch blk: {
                    printLastError(allocator, "rns.linkOpen after announce");
                    break :blk null;
                };
            } else if (kind == .link_failed) {
                std.debug.print("link failed while opening: {s}\n", .{rns.eventErrorMessage(&ev)});
            }
        } else |err| switch (err) {
            error.Timeout => {
                if (saw_announce or pathKnown(node, &dest_hash)) {
                    link = rns.linkOpen(node, &dest_hash) catch null;
                }
            },
            else => {
                printLastError(allocator, "rns.eventPoll failed");
                return 1;
            },
        }
        elapsed_ms += 200;
    }

    const opened = link orelse {
        std.debug.print("timed out before link open\n", .{});
        return 1;
    };
    defer rns.linkClose(opened) catch {};

    var established = false;
    while (elapsed_ms < deadline_ms and !established) {
        if (rns.eventPoll(node, 500, page_buf)) |ev| {
            switch (@as(rns.EventKind, @enumFromInt(ev.kind))) {
                .link_established => {
                    established = true;
                    std.debug.print("link established\n", .{});
                },
                .link_failed => {
                    std.debug.print("link establishment failed: {s}\n", .{rns.eventErrorMessage(&ev)});
                    return 1;
                },
                .link_closed => {
                    std.debug.print("link closed before establish\n", .{});
                    return 1;
                },
                else => {},
            }
        } else |err| switch (err) {
            error.Timeout => {},
            else => {
                printLastError(allocator, "rns.eventPoll failed");
                return 1;
            },
        }
        elapsed_ms += 500;
    }

    if (!established) {
        std.debug.print("timed out waiting for link establishment\n", .{});
        return 1;
    }

    const timeout_ms: c_int = @intCast(@max(deadline_ms - elapsed_ms, 1000));
    _ = rns.linkRequest(node, opened, page_path, &.{}, timeout_ms) catch {
        printLastError(allocator, "rns.linkRequest failed");
        return 1;
    };
    std.debug.print("request sent for {s}\n", .{page_path});

    while (elapsed_ms < deadline_ms) {
        if (rns.eventPoll(node, 500, page_buf)) |ev| {
            switch (@as(rns.EventKind, @enumFromInt(ev.kind))) {
                .request_response => {
                    const data = rns.eventAppData(&ev);
                    std.debug.print("\n=== Page Content ({d} bytes) ===\n", .{data.len});
                    if (data.len > 0) {
                        var stdout_buf: [1024]u8 = undefined;
                        var stdout_writer: Io.File.Writer = .init(.stdout(), io, &stdout_buf);
                        try stdout_writer.interface.writeAll(data);
                        if (data[data.len - 1] != '\n') try stdout_writer.interface.writeAll("\n");
                        try stdout_writer.interface.flush();
                    }
                    if (ev.app_data_truncated != 0) {
                        std.debug.print("warning: response truncated to {d} bytes\n", .{page_buf_cap});
                    }
                    std.debug.print("=== End of Page ===\n", .{});
                    return 0;
                },
                .request_failed => {
                    std.debug.print("request failed: {s}\n", .{rns.eventErrorMessage(&ev)});
                    return 1;
                },
                .link_closed => {
                    std.debug.print("link closed before response\n", .{});
                    return 1;
                },
                else => {},
            }
        } else |err| switch (err) {
            error.Timeout => {},
            else => {
                printLastError(allocator, "rns.eventPoll failed");
                return 1;
            },
        }
        elapsed_ms += 500;
    }

    std.debug.print("timed out waiting for page response\n", .{});
    return 1;
}

pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();
    const args = try init.minimal.args.toSlice(arena);

    var config_path: ?[]const u8 = null;
    var timeout_sec: i64 = default_timeout_sec;
    var target: ?[]const u8 = null;

    var i: usize = 1;
    while (i < args.len) : (i += 1) {
        const arg = args[i];
        if (std.mem.eql(u8, arg, "-c") and i + 1 < args.len) {
            i += 1;
            config_path = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-t") and i + 1 < args.len) {
            i += 1;
            timeout_sec = try std.fmt.parseInt(i64, args[i], 10);
            if (timeout_sec <= 0) {
                std.debug.print("timeout must be positive\n", .{});
                std.process.exit(1);
            }
            continue;
        }
        if (std.mem.eql(u8, arg, "-h") or std.mem.eql(u8, arg, "--help")) {
            usage(args[0]);
            return;
        }
        if (std.mem.startsWith(u8, arg, "-")) {
            std.debug.print("unknown option: {s}\n", .{arg});
            usage(args[0]);
            std.process.exit(1);
        }
        if (target != null) {
            std.debug.print("extra argument: {s}\n", .{arg});
            usage(args[0]);
            std.process.exit(1);
        }
        target = arg;
    }

    if (target == null or config_path == null) {
        usage(args[0]);
        std.process.exit(1);
    }

    const code = try run(init.gpa, init.io, config_path.?, target.?, timeout_sec);
    if (code != 0) std.process.exit(code);
}
