// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"

API_VERSION :: "1.5"
HASH_LEN :: 16

Node :: distinct u64
Identity :: distinct u64
Destination :: distinct u64
Link :: distinct u64

Error :: enum c.int {
	Ok              = 0,
	Invalid_Arg     = 1,
	Invalid_Handle  = 2,
	Not_Found       = 3,
	State           = 4,
	IO              = 5,
	Internal        = 6,
	Timeout         = 7,
	Truncated       = 8,
}

Event_Kind :: enum c.int {
	None               = 0,
	Announce           = 1,
	Link_Established   = 2,
	Link_Failed        = 3,
	Link_Data          = 4,
	Link_Closed        = 5,
	Request_Incoming   = 6,
	Request_Response   = 7,
	Request_Failed     = 8,
	Resource_Started   = 9,
	Resource_Concluded = 10,
	Destination_Data   = 11,
}

Event :: struct {
	kind:                   Event_Kind,
	link_id:                [HASH_LEN]u8,
	link_id_len:            c.size_t,
	destination_hash:       [HASH_LEN]u8,
	destination_hash_len:   c.size_t,
	identity_hash:          [HASH_LEN]u8,
	identity_hash_len:      c.size_t,
	request_id:             [HASH_LEN]u8,
	request_id_len:         c.size_t,
	hops:                   u8,
	path:                   [256]u8,
	path_truncated:         c.int,
	error_message:          [256]u8,
	error_message_truncated: c.int,
	app_data:               [^]u8,
	app_data_len:           c.size_t,
	app_data_cap:           c.size_t,
	app_data_truncated:     c.int,
}

Path_Entry :: struct {
	hash:      [HASH_LEN]u8,
	hash_len:  c.size_t,
	via:       [HASH_LEN]u8,
	via_len:   c.size_t,
	hops:      u8,
	iface:     [64]u8,
	timestamp: f64,
	expires:   f64,
}

Interface_Entry :: struct {
	name:       [96]u8,
	type_name:  [32]u8,
	online:     c.int,
	enabled:    c.int,
	rx_bytes:   u64,
	tx_bytes:   u64,
	rx_packets: u64,
	tx_packets: u64,
}

Event_Callback :: #type proc "c" (event: ^Event, user_data: rawptr)
