// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over the Zig librns bindings.
// Usage: zig-pageserver -c config [-i identity] [-a announce_sec] [-p page_file] [-P request_path]

const std = @import("std");
const Io = std.Io;
const rns = @import("rns");

const default_announce_sec: i64 = 900;
const default_page_path = "/page/index.mu";
const default_file_path = "/file/test.txt";
const default_page_file = "pages/index.mu";
const default_file_file = "files/test.txt";
const default_identity_path = "identity";
const req_data_cap = 64 * 1024;

const fallback_page =
    \\> Zig pageserver
    \\
    \\librns via Reticulum-Go
    \\
    \\Fallback page (file not found).
    \\
    \\`[Home`:/page/index.mu]
    \\`[Download Test File`:/file/test.txt]`_`f
    \\
    \\---
    \\
;

const fallback_file = "Test file from Reticulum-Go node!\n";

fn usage(argv0: []const u8) void {
    std.debug.print(
        \\Usage: {s} -c config [-i identity] [-a announce_sec] [-p page_file] [-f file] [-P request_path]
        \\
        \\Serve a NomadNet-compatible /page/ handler over librns.
        \\Destination: nomadnetwork.node
        \\Announce app_data name: librns-zig-pageserver
        \\
        \\Options:
        \\  -c path   Reticulum config file (required)
        \\  -i path   Persistent identity file (default {s})
        \\            Loaded when present, otherwise generated and saved
        \\  -a sec    Announce interval seconds (default {d}, 0 = once)
        \\  -p file   Micron page file to serve (default {s})
        \\  -f file   Download file to serve at /file/test.txt (default {s})
        \\  -P path   Request path to register (default {s})
        \\
    , .{ argv0, default_identity_path, default_announce_sec, default_page_file, default_file_file, default_page_path });
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

fn loadPage(allocator: std.mem.Allocator, io: Io, path: []const u8) ![]u8 {
    return Io.Dir.cwd().readFileAlloc(io, path, allocator, .unlimited) catch {
        std.debug.print("warning: could not read {s}, using built-in page\n", .{path});
        return try allocator.dupe(u8, fallback_page);
    };
}

fn loadFile(allocator: std.mem.Allocator, io: Io, path: []const u8) ![]u8 {
    return Io.Dir.cwd().readFileAlloc(io, path, allocator, .unlimited) catch {
        std.debug.print("warning: could not read {s}, using built-in file\n", .{path});
        return try allocator.dupe(u8, fallback_file);
    };
}

fn loadOrCreateIdentity(path: []const u8) rns.Error!rns.Identity {
    if (rns.identityLoad(path)) |identity| {
        std.debug.print("loaded identity from {s}\n", .{path});
        return identity;
    } else |_| {}

    const identity = try rns.identityGenerate();
    errdefer rns.identityDestroy(identity) catch {};
    try rns.identitySave(identity, path);
    std.debug.print("created and saved identity to {s}\n", .{path});
    return identity;
}

fn run(
    allocator: std.mem.Allocator,
    io: Io,
    config_path: []const u8,
    identity_path: []const u8,
    page_file: []const u8,
    file_file: []const u8,
    request_path: []const u8,
    file_path: []const u8,
    announce_sec: i64,
) !u8 {
    const ver = rns.version();
    if (!std.mem.eql(u8, ver, rns.api_version)) {
        std.debug.print("librns version mismatch: got {s} want {s}\n", .{ ver, rns.api_version });
        return 1;
    }

    const page_body = try loadPage(allocator, io, page_file);
    defer allocator.free(page_body);
    const file_body = try loadFile(allocator, io, file_file);
    defer allocator.free(file_body);

    const node = rns.nodeCreate(config_path) catch {
        printLastError(allocator, "rns.nodeCreate failed");
        return 1;
    };
    defer rns.nodeDestroy(node) catch {};

    const identity = loadOrCreateIdentity(identity_path) catch {
        printLastError(allocator, "identity load/create failed");
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

    const dest = rns.destinationCreate(node, @enumFromInt(0), "nomadnetwork", &.{"node"}, true) catch {
        printLastError(allocator, "rns.destinationCreate failed");
        return 1;
    };
    defer rns.destinationDestroy(dest) catch {};

    rns.destinationRegisterRequestHandler(dest, request_path) catch {
        printLastError(allocator, "rns.destinationRegisterRequestHandler failed");
        return 1;
    };
    rns.destinationRegisterRequestHandler(dest, file_path) catch {
        printLastError(allocator, "rns.destinationRegisterRequestHandler file failed");
        return 1;
    };

    const dest_hash = rns.destinationHash(dest) catch {
        printLastError(allocator, "rns.destinationHash failed");
        return 1;
    };
    var dest_hex_buf: [rns.hash_len * 2]u8 = undefined;
    const dest_hex = rns.hashToHex(&dest_hash, &dest_hex_buf) catch {
        std.debug.print("failed to encode destination hash\n", .{});
        return 1;
    };

    std.debug.print("DEST_HASH={s}\n", .{dest_hex});
    std.debug.print("REQUEST_PATH={s}\n", .{request_path});
    std.debug.print("FILE_PATH={s}\n", .{file_path});
    std.debug.print("librns {s} pageserver listening as nomadnetwork.node\n", .{ver});
    std.debug.print("announce name=librns-zig-pageserver interval={d}s\n", .{announce_sec});
    std.debug.print("serving {d} bytes from {s}\n", .{ page_body.len, page_file });
    std.debug.print("serving {d} bytes from {s} as {s}\n", .{ file_body.len, file_file, file_path });

    const app_data = "librns-zig-pageserver";
    if (rns.destinationAnnounce(dest, app_data)) |_| {
        std.debug.print("announce sent\n", .{});
    } else |_| {
        printLastError(allocator, "rns.destinationAnnounce failed");
    }

    const req_buf = try allocator.alloc(u8, req_data_cap);
    defer allocator.free(req_buf);

    var since_announce_ms: i64 = 0;
    const announce_every_ms = announce_sec * 1000;

    while (true) {
        if (announce_sec > 0 and since_announce_ms >= announce_every_ms) {
            if (rns.destinationAnnounce(dest, app_data)) |_| {
                std.debug.print("announce refreshed\n", .{});
            } else |_| {}
            since_announce_ms = 0;
        }

        if (rns.eventPoll(node, 200, req_buf)) |ev| {
            switch (@as(rns.EventKind, @enumFromInt(ev.kind))) {
                .link_established => std.debug.print("inbound link established\n", .{}),
                .link_closed => std.debug.print("link closed\n", .{}),
                .request_incoming => {
                    const path = rns.eventPath(&ev);
                    std.debug.print("request incoming path={s}\n", .{path});
                    const req_id = rns.eventRequestId(&ev);
                    if (std.mem.eql(u8, path, request_path)) {
                        if (rns.requestRespond(node, req_id, page_body)) |_| {
                            std.debug.print("served {s} ({d} bytes)\n", .{ request_path, page_body.len });
                        } else |_| {
                            printLastError(allocator, "rns.requestRespond failed");
                        }
                    } else if (std.mem.eql(u8, path, file_path)) {
                        if (rns.requestRespond(node, req_id, file_body)) |_| {
                            std.debug.print("served {s} ({d} bytes)\n", .{ file_path, file_body.len });
                        } else |_| {
                            printLastError(allocator, "rns.requestRespond failed");
                        }
                    } else {
                        rns.requestRespond(node, req_id, "page not found\n") catch {
                            printLastError(allocator, "rns.requestRespond failed");
                        };
                    }
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
        since_announce_ms += 200;
    }
}

pub fn main(init: std.process.Init) !void {
    const arena = init.arena.allocator();
    const args = try init.minimal.args.toSlice(arena);

    var config_path: ?[]const u8 = null;
    var identity_path: []const u8 = default_identity_path;
    var page_file: []const u8 = default_page_file;
    var file_file: []const u8 = default_file_file;
    var request_path: []const u8 = default_page_path;
    const file_path: []const u8 = default_file_path;
    var announce_sec: i64 = default_announce_sec;

    var i: usize = 1;
    while (i < args.len) : (i += 1) {
        const arg = args[i];
        if (std.mem.eql(u8, arg, "-c") and i + 1 < args.len) {
            i += 1;
            config_path = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-i") and i + 1 < args.len) {
            i += 1;
            identity_path = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-a") and i + 1 < args.len) {
            i += 1;
            announce_sec = try std.fmt.parseInt(i64, args[i], 10);
            if (announce_sec < 0) {
                std.debug.print("announce interval must be >= 0\n", .{});
                std.process.exit(1);
            }
            continue;
        }
        if (std.mem.eql(u8, arg, "-p") and i + 1 < args.len) {
            i += 1;
            page_file = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-f") and i + 1 < args.len) {
            i += 1;
            file_file = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-P") and i + 1 < args.len) {
            i += 1;
            request_path = args[i];
            continue;
        }
        if (std.mem.eql(u8, arg, "-h") or std.mem.eql(u8, arg, "--help")) {
            usage(args[0]);
            return;
        }
        std.debug.print("unknown option: {s}\n", .{arg});
        usage(args[0]);
        std.process.exit(1);
    }

    if (config_path == null) {
        usage(args[0]);
        std.process.exit(1);
    }

    const code = try run(init.gpa, init.io, config_path.?, identity_path, page_file, file_file, request_path, file_path, announce_sec);
    if (code != 0) std.process.exit(code);
}
