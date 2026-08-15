// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use crate::error::{map_code, Result};
use crate::ffi::{self, EventKind, HASH_LEN};
use crate::node::Node;
use crate::util;

pub struct Event {
    raw: ffi::Event,
    app_data: Vec<u8>,
    path: String,
    error_message: String,
}

impl Event {
    pub fn poll(node: &Node, timeout_ms: i32, app_data_buf: &mut [u8]) -> Result<Self> {
        let mut raw = unsafe { std::mem::zeroed::<ffi::Event>() };
        if !app_data_buf.is_empty() {
            raw.app_data = app_data_buf.as_mut_ptr();
            raw.app_data_cap = app_data_buf.len();
        }
        map_code(unsafe { ffi::rns_event_poll(node.handle(), &mut raw, timeout_ms) })?;
        let app_data = if raw.app_data.is_null() || raw.app_data_len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(raw.app_data, raw.app_data_len).to_vec() }
        };
        Ok(Self {
            path: util::cstr_field(&raw.path),
            error_message: util::cstr_field(&raw.error_message),
            raw,
            app_data,
        })
    }

    pub fn kind(&self) -> EventKind {
        match self.raw.kind {
            0 => EventKind::None,
            1 => EventKind::Announce,
            2 => EventKind::LinkEstablished,
            3 => EventKind::LinkFailed,
            4 => EventKind::LinkData,
            5 => EventKind::LinkClosed,
            6 => EventKind::RequestIncoming,
            7 => EventKind::RequestResponse,
            8 => EventKind::RequestFailed,
            9 => EventKind::ResourceStarted,
            10 => EventKind::ResourceConcluded,
            11 => EventKind::DestinationData,
            _ => EventKind::None,
        }
    }

    pub fn hops(&self) -> u8 {
        self.raw.hops
    }

    pub fn link_id(&self) -> &[u8] {
        &self.raw.link_id[..self.raw.link_id_len]
    }

    pub fn destination_hash(&self) -> &[u8] {
        &self.raw.destination_hash[..self.raw.destination_hash_len]
    }

    pub fn identity_hash(&self) -> &[u8] {
        &self.raw.identity_hash[..self.raw.identity_hash_len]
    }

    pub fn request_id(&self) -> &[u8] {
        &self.raw.request_id[..self.raw.request_id_len]
    }

    pub fn path(&self) -> &str {
        &self.path
    }

    pub fn error_message(&self) -> &str {
        &self.error_message
    }

    pub fn app_data(&self) -> &[u8] {
        &self.app_data
    }

    pub fn path_truncated(&self) -> bool {
        self.raw.path_truncated != 0
    }

    pub fn error_message_truncated(&self) -> bool {
        self.raw.error_message_truncated != 0
    }

    pub fn app_data_truncated(&self) -> bool {
        self.raw.app_data_truncated != 0
    }
}

pub fn hash_eq(a: &[u8], b: &[u8; HASH_LEN]) -> bool {
    a.len() == HASH_LEN && a == b.as_slice()
}
