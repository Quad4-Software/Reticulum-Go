// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

const c = @import("c.zig");

pub const api_version = "1.3";
pub const hash_len = c.RNS_HASH_LEN;

pub const Node = enum(u64) { _ };
pub const Identity = enum(u64) { _ };
pub const Destination = enum(u64) { _ };
pub const Link = enum(u64) { _ };

pub const Error = error{
    InvalidArg,
    InvalidHandle,
    NotFound,
    State,
    Io,
    Internal,
    Timeout,
    Truncated,
};

pub const EventKind = enum(c_int) {
    none = 0,
    announce = c.RNS_EV_ANNOUNCE,
    link_established = c.RNS_EV_LINK_ESTABLISHED,
    link_failed = c.RNS_EV_LINK_FAILED,
    link_data = c.RNS_EV_LINK_DATA,
    link_closed = c.RNS_EV_LINK_CLOSED,
    request_incoming = c.RNS_EV_REQUEST_INCOMING,
    request_response = c.RNS_EV_REQUEST_RESPONSE,
    request_failed = c.RNS_EV_REQUEST_FAILED,
    resource_started = c.RNS_EV_RESOURCE_STARTED,
    resource_concluded = c.RNS_EV_RESOURCE_CONCLUDED,
};

pub const Event = c.Event;
pub const PathEntry = c.PathEntry;
pub const EventCallback = c.EventCallback;

pub fn mapCode(code: c_int) Error!void {
    switch (code) {
        c.RNS_OK => {},
        c.RNS_ERR_INVALID_ARG => return error.InvalidArg,
        c.RNS_ERR_INVALID_HANDLE => return error.InvalidHandle,
        c.RNS_ERR_NOT_FOUND => return error.NotFound,
        c.RNS_ERR_STATE => return error.State,
        c.RNS_ERR_IO => return error.Io,
        c.RNS_ERR_INTERNAL => return error.Internal,
        c.RNS_ERR_TIMEOUT => return error.Timeout,
        c.RNS_ERR_TRUNCATED => return error.Truncated,
        else => return error.Internal,
    }
}

pub fn asU64(handle: anytype) u64 {
    return @intFromEnum(handle);
}
