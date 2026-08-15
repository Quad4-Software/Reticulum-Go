// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ffi::CString;
use std::ptr;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};
use crate::identity::Identity;
use crate::node::Node;

pub struct Destination {
    handle: u64,
}

impl Destination {
    pub fn create(
        node: &Node,
        identity: Option<&Identity>,
        app_name: &str,
        aspects: &[&str],
        accepts_links: bool,
    ) -> Result<Self> {
        if app_name.is_empty() {
            return Err(Error::InvalidArg);
        }
        let app = CString::new(app_name).map_err(|_| Error::InvalidArg)?;
        let mut aspect_ptrs: Vec<*const i8> = Vec::with_capacity(aspects.len());
        let mut aspect_strings: Vec<CString> = Vec::with_capacity(aspects.len());
        for aspect in aspects {
            aspect_strings.push(CString::new(*aspect).map_err(|_| Error::InvalidArg)?);
            aspect_ptrs.push(aspect_strings.last().unwrap().as_ptr());
        }
        let id_handle = identity.map(|i| i.handle()).unwrap_or(0);
        let h = unsafe {
            ffi::rns_destination_create(
                node.handle(),
                id_handle,
                app.as_ptr(),
                if aspect_ptrs.is_empty() {
                    ptr::null()
                } else {
                    aspect_ptrs.as_ptr()
                },
                aspect_ptrs.len(),
                if accepts_links { 1 } else { 0 },
            )
        };
        if h == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle: h })
    }

    pub fn enable_ratchets(&self, path: Option<&str>) -> Result<()> {
        match path {
            None => map_code(unsafe { ffi::rns_destination_enable_ratchets(self.handle, ptr::null()) }),
            Some(p) => {
                let c = CString::new(p).map_err(|_| Error::InvalidArg)?;
                map_code(unsafe { ffi::rns_destination_enable_ratchets(self.handle, c.as_ptr()) })
            }
        }
    }

    pub fn enforce_ratchets(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_destination_enforce_ratchets(self.handle) })
    }

    pub fn announce(&self, app_data: &[u8]) -> Result<()> {
        map_code(unsafe {
            ffi::rns_destination_announce(
                self.handle,
                if app_data.is_empty() {
                    ptr::null()
                } else {
                    app_data.as_ptr()
                },
                app_data.len(),
            )
        })
    }

    pub fn hash(&self) -> Result<[u8; HASH_LEN]> {
        let mut out = [0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_destination_hash(self.handle, out.as_mut_ptr(), out.len(), &mut written)
        })?;
        if written != HASH_LEN {
            return Err(Error::Truncated);
        }
        Ok(out)
    }

    pub fn register_request_handler(&self, path: &str) -> Result<()> {
        if path.is_empty() {
            return Err(Error::InvalidArg);
        }
        let path = CString::new(path).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rns_destination_register_request_handler(self.handle, path.as_ptr()) })
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Destination {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rns_destination_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
