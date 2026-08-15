// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, PathEntry, HASH_LEN};
use crate::node::Node;
use crate::util;

#[derive(Debug, Clone)]
pub struct PathInfo {
    pub hash: [u8; HASH_LEN],
    pub via: [u8; HASH_LEN],
    pub hops: u8,
    pub iface: String,
    pub timestamp: f64,
    pub expires: f64,
}

pub fn path_request(node: &Node, dest_hash: &[u8; HASH_LEN]) -> Result<()> {
    map_code(unsafe { ffi::rns_path_request(node.handle(), dest_hash.as_ptr()) })
}

pub fn path_table(node: &Node, capacity: usize, max_hops: i32) -> Result<Vec<PathInfo>> {
    if capacity == 0 {
        return Err(Error::InvalidArg);
    }
    let mut raw = vec![unsafe { std::mem::zeroed::<PathEntry>() }; capacity];
    let mut written = 0usize;
    let code = unsafe {
        ffi::rns_path_table(
            node.handle(),
            raw.as_mut_ptr(),
            raw.len(),
            &mut written,
            max_hops,
        )
    };
    if code != 0 && code != 8 {
        map_code(code)?;
    }
    let mut out = Vec::with_capacity(written);
    for entry in raw.into_iter().take(written) {
        let mut hash = [0u8; HASH_LEN];
        let mut via = [0u8; HASH_LEN];
        let hlen = entry.hash_len.min(HASH_LEN);
        let vlen = entry.via_len.min(HASH_LEN);
        hash[..hlen].copy_from_slice(&entry.hash[..hlen]);
        via[..vlen].copy_from_slice(&entry.via[..vlen]);
        out.push(PathInfo {
            hash,
            via,
            hops: entry.hops,
            iface: util::cstr_field(&entry.iface),
            timestamp: entry.timestamp,
            expires: entry.expires,
        });
    }
    Ok(out)
}

pub fn path_known(node: &Node, dest_hash: &[u8; HASH_LEN]) -> bool {
    path_table(node, 256, -1)
        .map(|table| table.iter().any(|e| e.hash == *dest_hash))
        .unwrap_or(false)
}
