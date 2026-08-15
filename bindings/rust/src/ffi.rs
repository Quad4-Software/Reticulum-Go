// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

#![allow(non_camel_case_types)]

use std::os::raw::{c_char, c_int, c_void};

pub const HASH_LEN: usize = 16;

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Event {
    pub kind: c_int,
    pub link_id: [u8; HASH_LEN],
    pub link_id_len: usize,
    pub destination_hash: [u8; HASH_LEN],
    pub destination_hash_len: usize,
    pub identity_hash: [u8; HASH_LEN],
    pub identity_hash_len: usize,
    pub request_id: [u8; HASH_LEN],
    pub request_id_len: usize,
    pub hops: u8,
    pub path: [c_char; 256],
    pub path_truncated: c_int,
    pub error_message: [c_char; 256],
    pub error_message_truncated: c_int,
    pub app_data: *mut u8,
    pub app_data_len: usize,
    pub app_data_cap: usize,
    pub app_data_truncated: c_int,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct PathEntry {
    pub hash: [u8; HASH_LEN],
    pub hash_len: usize,
    pub via: [u8; HASH_LEN],
    pub via_len: usize,
    pub hops: u8,
    pub iface: [c_char; 64],
    pub timestamp: f64,
    pub expires: f64,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct InterfaceEntry {
    pub name: [c_char; 96],
    pub type_name: [c_char; 32],
    pub online: c_int,
    pub enabled: c_int,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub rx_packets: u64,
    pub tx_packets: u64,
}

#[repr(i32)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EventKind {
    None = 0,
    Announce = 1,
    LinkEstablished = 2,
    LinkFailed = 3,
    LinkData = 4,
    LinkClosed = 5,
    RequestIncoming = 6,
    RequestResponse = 7,
    RequestFailed = 8,
    ResourceStarted = 9,
    ResourceConcluded = 10,
    DestinationData = 11,
}

unsafe extern "C" {
    pub fn rns_version() -> *const c_char;
    pub fn rns_last_error(buf: *mut c_char, buf_len: usize, written: *mut usize) -> c_int;

    pub fn rns_node_create(config_path: *const c_char) -> u64;
    pub fn rns_node_start(node: u64) -> c_int;
    pub fn rns_node_stop(node: u64) -> c_int;
    pub fn rns_node_destroy(node: u64) -> c_int;
    pub fn rns_node_set_identity(node: u64, identity: u64) -> c_int;
    pub fn rns_node_resume(node: u64) -> c_int;
    pub fn rns_node_pause(node: u64) -> c_int;
    pub fn rns_node_refresh_paths(node: u64, dest_hashes: *const u8, count: usize) -> c_int;

    pub fn rns_identity_generate() -> u64;
    pub fn rns_identity_load(path: *const c_char) -> u64;
    pub fn rns_identity_save(identity: u64, path: *const c_char) -> c_int;
    pub fn rns_identity_destroy(identity: u64) -> c_int;
    pub fn rns_identity_hash(
        identity: u64,
        hex_buf: *mut c_char,
        hex_buf_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_identity_hash_bytes(
        identity: u64,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_identity_public_key(
        identity: u64,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_identity_from_public_key(pub_key: *const u8, pub_len: usize) -> u64;
    pub fn rns_identity_sign(
        identity: u64,
        data: *const u8,
        data_len: usize,
        sig_out: *mut u8,
        sig_out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_identity_verify(
        identity: u64,
        data: *const u8,
        data_len: usize,
        sig: *const u8,
        sig_len: usize,
    ) -> c_int;

    pub fn rns_rsg_create(
        identity: u64,
        message: *const u8,
        message_len: usize,
        embed: c_int,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_rsg_validate(
        rsg: *const u8,
        rsg_len: usize,
        message: *const u8,
        message_len: usize,
        required_signer_hash: *const u8,
        required_signer_hash_len: usize,
    ) -> c_int;
    pub fn rns_rsg_sign_file(
        identity: u64,
        path: *const c_char,
        out: *mut u8,
        out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_rsg_verify_file(
        rsg: *const u8,
        rsg_len: usize,
        path: *const c_char,
        required_signer_hash: *const u8,
        required_signer_hash_len: usize,
    ) -> c_int;
    pub fn rns_rsm_verify(
        rsm: *const u8,
        rsm_len: usize,
        required_signer_hash: *const u8,
        required_signer_hash_len: usize,
        message_out: *mut u8,
        message_out_len: usize,
        written: *mut usize,
    ) -> c_int;

    pub fn rns_destination_create(
        node: u64,
        identity: u64,
        app_name: *const c_char,
        aspects: *const *const c_char,
        aspect_count: usize,
        accepts_links: c_int,
    ) -> u64;
    pub fn rns_destination_enable_ratchets(destination: u64, path: *const c_char) -> c_int;
    pub fn rns_destination_enforce_ratchets(destination: u64) -> c_int;
    pub fn rns_destination_announce(
        destination: u64,
        app_data: *const u8,
        app_data_len: usize,
    ) -> c_int;
    pub fn rns_destination_hash(
        destination: u64,
        hash_out: *mut u8,
        hash_out_len: usize,
        written: *mut usize,
    ) -> c_int;
    pub fn rns_destination_destroy(destination: u64) -> c_int;
    pub fn rns_destination_register_request_handler(
        destination: u64,
        path: *const c_char,
    ) -> c_int;

    pub fn rns_path_request(node: u64, dest_hash: *const u8) -> c_int;
    pub fn rns_path_table(
        node: u64,
        out: *mut PathEntry,
        out_cap: usize,
        written: *mut usize,
        max_hops: c_int,
    ) -> c_int;
    pub fn rns_interfaces(
        node: u64,
        out: *mut InterfaceEntry,
        out_cap: usize,
        written: *mut usize,
    ) -> c_int;

    pub fn rns_link_open(node: u64, dest_hash: *const u8) -> u64;
    pub fn rns_link_send(link: u64, data: *const u8, data_len: usize) -> c_int;
    pub fn rns_link_send_resource(
        link: u64,
        data: *const u8,
        data_len: usize,
        name: *const c_char,
    ) -> c_int;
    pub fn rns_link_close(link: u64) -> c_int;
    pub fn rns_link_id(link: u64, id_out: *mut u8, id_out_len: usize, written: *mut usize)
        -> c_int;
    pub fn rns_link_request(
        node: u64,
        link: u64,
        path: *const c_char,
        data: *const u8,
        data_len: usize,
        timeout_ms: c_int,
        request_id_out: *mut u8,
        request_id_out_len: usize,
        written: *mut usize,
    ) -> c_int;

    pub fn rns_request_respond(
        node: u64,
        request_id: *const u8,
        request_id_len: usize,
        data: *const u8,
        data_len: usize,
    ) -> c_int;
    pub fn rns_request_respond_file(
        node: u64,
        request_id: *const u8,
        request_id_len: usize,
        filename: *const c_char,
        data: *const u8,
        data_len: usize,
    ) -> c_int;

    pub fn rns_event_poll(node: u64, event: *mut Event, timeout_ms: c_int) -> c_int;
    pub fn rns_set_event_callback(
        node: u64,
        callback: Option<unsafe extern "C" fn(*const Event, *mut c_void)>,
        user_data: *mut c_void,
    ) -> c_int;
}
