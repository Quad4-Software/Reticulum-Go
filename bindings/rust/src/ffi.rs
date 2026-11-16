// SPDX-License-Identifier: LicenseRef-Reticulum

#![allow(non_camel_case_types)]

use std::os::raw::{c_char, c_int};

pub const HASH_LEN: usize = 16;

#[repr(C)]
#[derive(Clone, Copy)]
pub struct RrcEvent {
    pub kind: c_int,
    pub sender: [u8; HASH_LEN],
    pub sender_len: usize,
    pub peer: [u8; HASH_LEN],
    pub peer_len: usize,
    pub room: [c_char; 128],
    pub room_truncated: c_int,
    pub nick: [c_char; 64],
    pub nick_truncated: c_int,
    pub body: [c_char; 1024],
    pub body_truncated: c_int,
    pub msg_type: u64,
}

pub const RRC_TYPE_MSG: u64 = 20;

unsafe extern "C" {
    pub fn rrc_version() -> *const c_char;
    pub fn rrc_last_error(buf: *mut c_char, buf_len: usize, written: *mut usize) -> c_int;

    pub fn rrc_envelope_create(msg_type: u64, sender: *const u8, sender_len: usize) -> u64;
    pub fn rrc_envelope_set_room(envelope: u64, room: *const c_char) -> c_int;
    pub fn rrc_envelope_set_nick(envelope: u64, nick: *const c_char) -> c_int;
    pub fn rrc_envelope_set_body_text(envelope: u64, text: *const c_char) -> c_int;
    pub fn rrc_envelope_set_destination(envelope: u64, dest: *const u8, dest_len: usize) -> c_int;
    pub fn rrc_envelope_get_type(envelope: u64, out: *mut u64) -> c_int;
    pub fn rrc_envelope_get_sender(
        envelope: u64,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_envelope_get_room(
        envelope: u64,
        buf: *mut c_char,
        buf_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_envelope_get_nick(
        envelope: u64,
        buf: *mut c_char,
        buf_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_envelope_get_body_text(
        envelope: u64,
        buf: *mut c_char,
        buf_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_envelope_marshal(
        envelope: u64,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_envelope_unmarshal(data: *const u8, data_len: usize) -> u64;
    pub fn rrc_envelope_destroy(envelope: u64) -> c_int;

    pub fn rrc_normalize_room(
        input: *const c_char,
        out: *mut c_char,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_sanitize_nick(
        input: *const c_char,
        out: *mut c_char,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;

    pub fn rrc_node_create(config_path: *const c_char) -> u64;
    pub fn rrc_node_start(node: u64) -> c_int;
    pub fn rrc_node_stop(node: u64) -> c_int;
    pub fn rrc_node_destroy(node: u64) -> c_int;
    pub fn rrc_node_set_identity(node: u64, identity: u64) -> c_int;
    pub fn rrc_node_add_udp_interface(
        node: u64,
        name: *const c_char,
        local_addr: *const c_char,
        peer_addr: *const c_char,
    ) -> c_int;
    pub fn rrc_node_add_tcp_client_interface(
        node: u64,
        name: *const c_char,
        target: *const c_char,
    ) -> c_int;
    pub fn rrc_node_add_tcp_server_interface(
        node: u64,
        name: *const c_char,
        listen: *const c_char,
    ) -> c_int;
    pub fn rrc_node_add_auto_interface(node: u64, name: *const c_char) -> c_int;
    pub fn rrc_node_add_backbone_client_interface(
        node: u64,
        name: *const c_char,
        target: *const c_char,
    ) -> c_int;
    pub fn rrc_node_interface_count(node: u64) -> c_int;
    pub fn rrc_node_interface_names(
        node: u64,
        out: *mut c_char,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_node_has_path(
        node: u64,
        dest_hash: *const u8,
        dest_hash_len: usize,
        has_path: *mut c_int,
    ) -> c_int;

    pub fn rrc_identity_generate() -> u64;
    pub fn rrc_identity_load(path: *const c_char) -> u64;
    pub fn rrc_identity_save(identity: u64, path: *const c_char) -> c_int;
    pub fn rrc_identity_destroy(identity: u64) -> c_int;
    pub fn rrc_identity_hash(
        identity: u64,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rrc_identity_seed_destination(
        identity: u64,
        dest_hash: *const u8,
        dest_hash_len: usize,
    ) -> c_int;

    pub fn rrc_hub_create(node: u64, identity: u64, name: *const c_char, version: *const c_char) -> u64;
    pub fn rrc_hub_start(hub: u64) -> c_int;
    pub fn rrc_hub_announce(hub: u64) -> c_int;
    pub fn rrc_hub_hash(hub: u64, out: *mut u8, out_len: usize, written: *mut usize) -> c_int;
    pub fn rrc_hub_peer_count(hub: u64, count: *mut usize) -> c_int;
    pub fn rrc_hub_destroy(hub: u64) -> c_int;
    pub fn rrc_hub_event_poll(hub: u64, timeout_ms: c_int, event: *mut RrcEvent) -> c_int;

    pub fn rrc_client_dial(
        node: u64,
        identity: u64,
        hub_hash: *const u8,
        hub_hash_len: usize,
        nick: *const c_char,
        name: *const c_char,
        version: *const c_char,
        timeout_ms: c_int,
    ) -> u64;
    pub fn rrc_client_join(client: u64, room: *const c_char) -> c_int;
    pub fn rrc_client_part(client: u64, room: *const c_char) -> c_int;
    pub fn rrc_client_send_msg(client: u64, room: *const c_char, text: *const c_char) -> c_int;
    pub fn rrc_client_send_notice(client: u64, room: *const c_char, text: *const c_char) -> c_int;
    pub fn rrc_client_send_action(client: u64, room: *const c_char, text: *const c_char) -> c_int;
    pub fn rrc_client_ping(client: u64) -> c_int;
    pub fn rrc_client_close(client: u64) -> c_int;
    pub fn rrc_client_event_poll(client: u64, timeout_ms: c_int, event: *mut RrcEvent) -> c_int;
}
