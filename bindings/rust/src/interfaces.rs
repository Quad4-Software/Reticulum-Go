// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, InterfaceEntry};
use crate::node::Node;

#[derive(Debug, Clone)]
pub struct InterfaceInfo {
    pub name: String,
    pub type_name: String,
    pub online: bool,
    pub enabled: bool,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub rx_packets: u64,
    pub tx_packets: u64,
}

fn cstr_field(buf: &[i8]) -> String {
    let bytes: Vec<u8> = buf
        .iter()
        .map(|b| *b as u8)
        .take_while(|b| *b != 0)
        .collect();
    String::from_utf8_lossy(&bytes).into_owned()
}

pub fn interfaces_list(node: &Node, capacity: usize) -> Result<Vec<InterfaceInfo>> {
    if capacity == 0 {
        return Err(Error::InvalidArg);
    }
    let mut raw = vec![unsafe { std::mem::zeroed::<InterfaceEntry>() }; capacity];
    let mut written = 0usize;
    let code = unsafe { ffi::rns_interfaces(node.handle(), raw.as_mut_ptr(), raw.len(), &mut written) };
    if code != 0 && code != 8 {
        map_code(code)?;
    }
    let mut out = Vec::with_capacity(written);
    for entry in raw.into_iter().take(written) {
        out.push(InterfaceInfo {
            name: cstr_field(&entry.name),
            type_name: cstr_field(&entry.type_name),
            online: entry.online != 0,
            enabled: entry.enabled != 0,
            rx_bytes: entry.rx_bytes,
            tx_bytes: entry.tx_bytes,
            rx_packets: entry.rx_packets,
            tx_packets: entry.tx_packets,
        });
    }
    Ok(out)
}
