// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"

event_poll :: proc(node: Node, timeout_ms: i32, app_data_buf: []u8 = nil) -> (event: Event, err: Error) {
	if len(app_data_buf) > 0 {
		event.app_data = raw_data(app_data_buf)
		event.app_data_cap = c.size_t(len(app_data_buf))
	}
	code := Error(rns_event_poll(u64(node), &event, c.int(timeout_ms)))
	return event, code
}

set_event_callback :: proc(node: Node, callback: Event_Callback, user_data: rawptr = nil) -> Error {
	return Error(rns_set_event_callback(u64(node), callback, user_data))
}

event_app_data :: proc(event: ^Event) -> []u8 {
	if event.app_data == nil || event.app_data_len == 0 {
		return nil
	}
	return event.app_data[:event.app_data_len]
}

event_path :: proc(event: ^Event) -> string {
	return cstring_field(event.path[:])
}

event_error_message :: proc(event: ^Event) -> string {
	return cstring_field(event.error_message[:])
}

event_link_id :: proc(event: ^Event) -> []u8 {
	return event.link_id[:event.link_id_len]
}

event_destination_hash :: proc(event: ^Event) -> []u8 {
	return event.destination_hash[:event.destination_hash_len]
}

event_identity_hash :: proc(event: ^Event) -> []u8 {
	return event.identity_hash[:event.identity_hash_len]
}

event_request_id :: proc(event: ^Event) -> []u8 {
	return event.request_id[:event.request_id_len]
}
