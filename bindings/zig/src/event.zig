// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const std = @import("std");
const c = @import("c.zig");
const types = @import("types.zig");
const util = @import("util.zig");

pub fn poll(node: types.Node, timeout_ms: c_int, app_data_buf: []u8) types.Error!types.Event {
    var event: types.Event = std.mem.zeroes(types.Event);
    if (app_data_buf.len > 0) {
        event.app_data = app_data_buf.ptr;
        event.app_data_cap = app_data_buf.len;
    }
    try types.mapCode(c.rns_event_poll(types.asU64(node), &event, timeout_ms));
    return event;
}

pub fn setCallback(node: types.Node, callback: ?types.EventCallback, user_data: ?*anyopaque) types.Error!void {
    try types.mapCode(c.rns_set_event_callback(types.asU64(node), callback, user_data));
}

pub fn appData(event: *const types.Event) []u8 {
    if (event.app_data == null or event.app_data_len == 0) return &.{};
    return event.app_data.?[0..event.app_data_len];
}

pub fn path(event: *const types.Event) []const u8 {
    return util.cstringField(&event.path);
}

pub fn errorMessage(event: *const types.Event) []const u8 {
    return util.cstringField(&event.error_message);
}

pub fn linkId(event: *const types.Event) []const u8 {
    return event.link_id[0..event.link_id_len];
}

pub fn destinationHash(event: *const types.Event) []const u8 {
    return event.destination_hash[0..event.destination_hash_len];
}

pub fn identityHash(event: *const types.Event) []const u8 {
    return event.identity_hash[0..event.identity_hash_len];
}

pub fn requestId(event: *const types.Event) []const u8 {
    return event.request_id[0..event.request_id_len];
}
