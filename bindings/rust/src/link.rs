// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ffi::CString;
use std::ptr;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};
use crate::node::Node;

pub struct Link {
    handle: u64,
}

impl Link {
    pub fn open(node: &Node, dest_hash: &[u8; HASH_LEN]) -> Result<Self> {
        let h = unsafe { ffi::rns_link_open(node.handle(), dest_hash.as_ptr()) };
        if h == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle: h })
    }

    pub fn send(&self, data: &[u8]) -> Result<()> {
        if data.is_empty() {
            return Err(Error::InvalidArg);
        }
        map_code(unsafe { ffi::rns_link_send(self.handle, data.as_ptr(), data.len()) })
    }

    pub fn send_resource(&self, data: &[u8], name: &str) -> Result<()> {
        let name = if name.is_empty() {
            None
        } else {
            Some(CString::new(name).map_err(|_| Error::InvalidArg)?)
        };
        map_code(unsafe {
            ffi::rns_link_send_resource(
                self.handle,
                if data.is_empty() {
                    ptr::null()
                } else {
                    data.as_ptr()
                },
                data.len(),
                name.as_ref().map_or(ptr::null(), |n| n.as_ptr()),
            )
        })
    }

    pub fn close(&self) -> Result<()> {
        map_code(unsafe { ffi::rns_link_close(self.handle) })
    }

    pub fn id(&self) -> Result<[u8; HASH_LEN]> {
        let mut out = [0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_link_id(self.handle, out.as_mut_ptr(), out.len(), &mut written)
        })?;
        if written != HASH_LEN {
            return Err(Error::Truncated);
        }
        Ok(out)
    }

    pub fn request(
        &self,
        node: &Node,
        path: &str,
        data: &[u8],
        timeout_ms: i32,
    ) -> Result<[u8; HASH_LEN]> {
        if path.is_empty() {
            return Err(Error::InvalidArg);
        }
        let path = CString::new(path).map_err(|_| Error::InvalidArg)?;
        let mut request_id = [0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_link_request(
                node.handle(),
                self.handle,
                path.as_ptr(),
                if data.is_empty() {
                    ptr::null()
                } else {
                    data.as_ptr()
                },
                data.len(),
                timeout_ms,
                request_id.as_mut_ptr(),
                request_id.len(),
                &mut written,
            )
        })?;
        if written != HASH_LEN {
            return Err(Error::Truncated);
        }
        Ok(request_id)
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Link {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rns_link_close(self.handle);
            }
            self.handle = 0;
        }
    }
}

pub fn request_respond(node: &Node, request_id: &[u8], data: &[u8]) -> Result<()> {
    if request_id.is_empty() {
        return Err(Error::InvalidArg);
    }
    map_code(unsafe {
        ffi::rns_request_respond(
            node.handle(),
            request_id.as_ptr(),
            request_id.len(),
            if data.is_empty() {
                ptr::null()
            } else {
                data.as_ptr()
            },
            data.len(),
        )
    })
}

pub fn request_respond_file(
    node: &Node,
    request_id: &[u8],
    filename: &str,
    data: &[u8],
) -> Result<()> {
    if request_id.is_empty() || filename.is_empty() {
        return Err(Error::InvalidArg);
    }
    let filename = CString::new(filename).map_err(|_| Error::InvalidArg)?;
    map_code(unsafe {
        ffi::rns_request_respond_file(
            node.handle(),
            request_id.as_ptr(),
            request_id.len(),
            filename.as_ptr(),
            if data.is_empty() {
                ptr::null()
            } else {
                data.as_ptr()
            },
            data.len(),
        )
    })
}
