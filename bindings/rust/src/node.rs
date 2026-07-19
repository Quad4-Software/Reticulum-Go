// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ffi::CString;
use std::ptr;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};
use crate::identity::Identity;

pub struct Node {
    handle: u64,
}

impl Node {
    pub fn create(config_path: &str) -> Result<Self> {
        let path = if config_path.is_empty() {
            CString::new("").map_err(|_| Error::InvalidArg)?
        } else {
            CString::new(config_path).map_err(|_| Error::InvalidArg)?
        };
        let h = unsafe { ffi::rns_node_create(path.as_ptr()) };
        if h == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle: h })
    }

    pub fn start(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_node_start(self.handle) })
    }

    pub fn stop(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_node_stop(self.handle) })
    }

    pub fn set_identity(&self, identity: &Identity) -> Result<()> {
        map_code(unsafe { ffi::rns_node_set_identity(self.handle, identity.handle()) })
    }

    pub fn pause(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_node_pause(self.handle) })
    }

    pub fn resume(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_node_resume(self.handle) })
    }

    pub fn event_poll(&self, timeout_ms: i32) -> Result<crate::event::Event> {
        let mut buf = vec![0u8; 65536];
        crate::event::Event::poll(self, timeout_ms, &mut buf)
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Node {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rns_node_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}

pub fn refresh_paths(node: &Node, dest_hashes: &[[u8; HASH_LEN]]) -> Result<()> {
    if dest_hashes.is_empty() {
        return map_code(unsafe { ffi::rns_node_refresh_paths(node.handle(), ptr::null(), 0) });
    }
    let flat: Vec<u8> = dest_hashes.iter().flat_map(|h| h.iter().copied()).collect();
    map_code(unsafe {
        ffi::rns_node_refresh_paths(node.handle(), flat.as_ptr(), dest_hashes.len())
    })
}
