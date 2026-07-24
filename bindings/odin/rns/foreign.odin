// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns

import "core:c"

when ODIN_OS == .Linux || ODIN_OS == .Darwin {
	foreign import lib "system:rns"
} else when ODIN_OS == .Windows {
	foreign import lib "system:rns"
} else {
	#panic("odin rns bindings require Linux, Darwin, or Windows librns")
}

@(default_calling_convention = "c")
foreign lib {
	rns_version :: proc() -> cstring ---

	rns_last_error :: proc(buf: [^]u8, buf_len: c.size_t, written: ^c.size_t) -> c.int ---

	rns_node_create         :: proc(config_path: cstring) -> u64 ---
	rns_node_start          :: proc(node: u64) -> c.int ---
	rns_node_stop           :: proc(node: u64) -> c.int ---
	rns_node_destroy        :: proc(node: u64) -> c.int ---
	rns_node_set_identity   :: proc(node: u64, identity: u64) -> c.int ---
	rns_node_resume         :: proc(node: u64) -> c.int ---
	rns_node_pause          :: proc(node: u64) -> c.int ---
	rns_node_refresh_paths  :: proc(node: u64, dest_hashes: [^]u8, count: c.size_t) -> c.int ---

	rns_identity_generate :: proc() -> u64 ---
	rns_identity_load     :: proc(path: cstring) -> u64 ---
	rns_identity_save     :: proc(identity: u64, path: cstring) -> c.int ---
	rns_identity_destroy  :: proc(identity: u64) -> c.int ---
	rns_identity_hash     :: proc(identity: u64, hex_buf: [^]u8, hex_buf_len: c.size_t, written: ^c.size_t) -> c.int ---
	rns_identity_hash_bytes :: proc(identity: u64, out: [^]u8, out_len: c.size_t, written: ^c.size_t) -> c.int ---
	rns_identity_public_key :: proc(identity: u64, out: [^]u8, out_len: c.size_t, written: ^c.size_t) -> c.int ---
	rns_identity_from_public_key :: proc(pub: [^]u8, pub_len: c.size_t) -> u64 ---
	rns_identity_sign :: proc(
		identity: u64,
		data: [^]u8,
		data_len: c.size_t,
		sig_out: [^]u8,
		sig_out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---
	rns_identity_verify :: proc(
		identity: u64,
		data: [^]u8,
		data_len: c.size_t,
		sig: [^]u8,
		sig_len: c.size_t,
	) -> c.int ---

	rns_rsg_create :: proc(
		identity: u64,
		message: [^]u8,
		message_len: c.size_t,
		embed: c.int,
		out: [^]u8,
		out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---
	rns_rsg_validate :: proc(
		rsg: [^]u8,
		rsg_len: c.size_t,
		message: [^]u8,
		message_len: c.size_t,
		required_signer_hash: [^]u8,
		required_signer_hash_len: c.size_t,
	) -> c.int ---
	rns_rsg_sign_file :: proc(
		identity: u64,
		path: cstring,
		out: [^]u8,
		out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---
	rns_rsg_verify_file :: proc(
		rsg: [^]u8,
		rsg_len: c.size_t,
		path: cstring,
		required_signer_hash: [^]u8,
		required_signer_hash_len: c.size_t,
	) -> c.int ---
	rns_rsm_verify :: proc(
		rsm: [^]u8,
		rsm_len: c.size_t,
		required_signer_hash: [^]u8,
		required_signer_hash_len: c.size_t,
		message_out: [^]u8,
		message_out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---

	rns_destination_create :: proc(
		node: u64,
		identity: u64,
		app_name: cstring,
		aspects: [^]cstring,
		aspect_count: c.size_t,
		accepts_links: c.int,
	) -> u64 ---
	rns_destination_announce :: proc(destination: u64, app_data: [^]u8, app_data_len: c.size_t) -> c.int ---
	rns_destination_hash :: proc(
		destination: u64,
		hash_out: [^]u8,
		hash_out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---
	rns_destination_destroy :: proc(destination: u64) -> c.int ---
	rns_destination_set_proof_strategy :: proc(destination: u64, strategy: c.int) -> c.int ---
	rns_destination_register_request_handler :: proc(destination: u64, path: cstring) -> c.int ---
	rns_destination_encrypt :: proc(
		dest_hash: [^]u8,
		plaintext: [^]u8,
		plaintext_len: c.size_t,
		out: [^]u8,
		out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---
	rns_packet_send :: proc(
		node: u64,
		dest_hash: [^]u8,
		plaintext: [^]u8,
		plaintext_len: c.size_t,
	) -> c.int ---

	rns_path_request :: proc(node: u64, dest_hash: [^]u8) -> c.int ---
	rns_path_table :: proc(
		node: u64,
		out: [^]Path_Entry,
		out_cap: c.size_t,
		written: ^c.size_t,
		max_hops: c.int,
	) -> c.int ---

	rns_interfaces :: proc(
		node: u64,
		out: [^]Interface_Entry,
		out_cap: c.size_t,
		written: ^c.size_t,
	) -> c.int ---

	rns_link_open :: proc(node: u64, dest_hash: [^]u8) -> u64 ---
	rns_link_send :: proc(link: u64, data: [^]u8, data_len: c.size_t) -> c.int ---
	rns_link_send_resource :: proc(link: u64, data: [^]u8, data_len: c.size_t, name: cstring) -> c.int ---
	rns_link_close :: proc(link: u64) -> c.int ---
	rns_link_id :: proc(link: u64, id_out: [^]u8, id_out_len: c.size_t, written: ^c.size_t) -> c.int ---
	rns_link_request :: proc(
		node: u64,
		link: u64,
		path: cstring,
		data: [^]u8,
		data_len: c.size_t,
		timeout_ms: c.int,
		request_id_out: [^]u8,
		request_id_out_len: c.size_t,
		written: ^c.size_t,
	) -> c.int ---

	rns_request_respond :: proc(
		node: u64,
		request_id: [^]u8,
		request_id_len: c.size_t,
		data: [^]u8,
		data_len: c.size_t,
	) -> c.int ---

	rns_request_respond_file :: proc(
		node: u64,
		request_id: [^]u8,
		request_id_len: c.size_t,
		filename: cstring,
		data: [^]u8,
		data_len: c.size_t,
	) -> c.int ---

	rns_event_poll :: proc(node: u64, event: ^Event, timeout_ms: c.int) -> c.int ---
	rns_set_event_callback :: proc(node: u64, callback: Event_Callback, user_data: rawptr) -> c.int ---
}
