// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const builtin = @import("builtin");

comptime {
    if (builtin.os.tag != .linux) {
        @compileError("zig rns bindings require Linux librns.so");
    }
}

pub const RNS_HASH_LEN = 16;

pub const RNS_OK: c_int = 0;
pub const RNS_ERR_INVALID_ARG: c_int = 1;
pub const RNS_ERR_INVALID_HANDLE: c_int = 2;
pub const RNS_ERR_NOT_FOUND: c_int = 3;
pub const RNS_ERR_STATE: c_int = 4;
pub const RNS_ERR_IO: c_int = 5;
pub const RNS_ERR_INTERNAL: c_int = 6;
pub const RNS_ERR_TIMEOUT: c_int = 7;
pub const RNS_ERR_TRUNCATED: c_int = 8;

pub const RNS_EV_ANNOUNCE: c_int = 1;
pub const RNS_EV_LINK_ESTABLISHED: c_int = 2;
pub const RNS_EV_LINK_FAILED: c_int = 3;
pub const RNS_EV_LINK_DATA: c_int = 4;
pub const RNS_EV_LINK_CLOSED: c_int = 5;
pub const RNS_EV_REQUEST_INCOMING: c_int = 6;
pub const RNS_EV_REQUEST_RESPONSE: c_int = 7;
pub const RNS_EV_REQUEST_FAILED: c_int = 8;

pub const Event = extern struct {
    kind: c_int,
    link_id: [RNS_HASH_LEN]u8,
    link_id_len: usize,
    destination_hash: [RNS_HASH_LEN]u8,
    destination_hash_len: usize,
    identity_hash: [RNS_HASH_LEN]u8,
    identity_hash_len: usize,
    request_id: [RNS_HASH_LEN]u8,
    request_id_len: usize,
    hops: u8,
    path: [256]u8,
    path_truncated: c_int,
    error_message: [256]u8,
    error_message_truncated: c_int,
    app_data: ?[*]u8,
    app_data_len: usize,
    app_data_cap: usize,
    app_data_truncated: c_int,
};

pub const PathEntry = extern struct {
    hash: [RNS_HASH_LEN]u8,
    hash_len: usize,
    via: [RNS_HASH_LEN]u8,
    via_len: usize,
    hops: u8,
    iface: [64]u8,
    timestamp: f64,
    expires: f64,
};

pub const EventCallback = *const fn (event: *const Event, user_data: ?*anyopaque) callconv(.c) void;

pub extern fn rns_version() ?[*:0]const u8;
pub extern fn rns_last_error(buf: ?[*]u8, buf_len: usize, written: ?*usize) c_int;

pub extern fn rns_node_create(config_path: ?[*:0]const u8) u64;
pub extern fn rns_node_start(node: u64) c_int;
pub extern fn rns_node_stop(node: u64) c_int;
pub extern fn rns_node_destroy(node: u64) c_int;
pub extern fn rns_node_set_identity(node: u64, identity: u64) c_int;
pub extern fn rns_node_resume(node: u64) c_int;
pub extern fn rns_node_pause(node: u64) c_int;
pub extern fn rns_node_refresh_paths(node: u64, dest_hashes: ?[*]const u8, count: usize) c_int;

pub extern fn rns_identity_generate() u64;
pub extern fn rns_identity_load(path: [*:0]const u8) u64;
pub extern fn rns_identity_save(identity: u64, path: [*:0]const u8) c_int;
pub extern fn rns_identity_destroy(identity: u64) c_int;
pub extern fn rns_identity_hash(identity: u64, hex_buf: ?[*]u8, hex_buf_len: usize, written: ?*usize) c_int;

pub extern fn rns_destination_create(
    node: u64,
    identity: u64,
    app_name: [*:0]const u8,
    aspects: ?[*]const [*:0]const u8,
    aspect_count: usize,
    accepts_links: c_int,
) u64;
pub extern fn rns_destination_announce(destination: u64, app_data: ?[*]const u8, app_data_len: usize) c_int;
pub extern fn rns_destination_hash(destination: u64, hash_out: ?[*]u8, hash_out_len: usize, written: ?*usize) c_int;
pub extern fn rns_destination_destroy(destination: u64) c_int;
pub extern fn rns_destination_register_request_handler(destination: u64, path: [*:0]const u8) c_int;

pub extern fn rns_path_request(node: u64, dest_hash: [*]const u8) c_int;
pub extern fn rns_path_table(node: u64, out: ?[*]PathEntry, out_cap: usize, written: ?*usize, max_hops: c_int) c_int;

pub extern fn rns_link_open(node: u64, dest_hash: [*]const u8) u64;
pub extern fn rns_link_send(link: u64, data: [*]const u8, data_len: usize) c_int;
pub extern fn rns_link_close(link: u64) c_int;
pub extern fn rns_link_id(link: u64, id_out: ?[*]u8, id_out_len: usize, written: ?*usize) c_int;
pub extern fn rns_link_request(
    node: u64,
    link: u64,
    path: [*:0]const u8,
    data: ?[*]const u8,
    data_len: usize,
    timeout_ms: c_int,
    request_id_out: ?[*]u8,
    request_id_out_len: usize,
    written: ?*usize,
) c_int;

pub extern fn rns_request_respond(
    node: u64,
    request_id: [*]const u8,
    request_id_len: usize,
    data: ?[*]const u8,
    data_len: usize,
) c_int;

pub extern fn rns_event_poll(node: u64, event: *Event, timeout_ms: c_int) c_int;
pub extern fn rns_set_event_callback(node: u64, callback: ?EventCallback, user_data: ?*anyopaque) c_int;
