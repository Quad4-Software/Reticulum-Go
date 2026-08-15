// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ptr;

use crate::error::{map_code, Error, Result};
use crate::ffi;
use crate::identity::Identity;

pub fn rsg_create(identity: &Identity, message: &[u8], embed: bool) -> Result<Vec<u8>> {
    let mut needed = 0usize;
    let probe = unsafe {
        ffi::rns_rsg_create(
            identity.handle(),
            if message.is_empty() {
                ptr::null()
            } else {
                message.as_ptr()
            },
            message.len(),
            if embed { 1 } else { 0 },
            ptr::null_mut(),
            0,
            &mut needed,
        )
    };
    if probe != 0 && probe != 8 {
        map_code(probe)?;
    }
    if needed == 0 {
        return Err(Error::Internal);
    }
    let mut out = vec![0u8; needed];
    let mut written = 0usize;
    map_code(unsafe {
        ffi::rns_rsg_create(
            identity.handle(),
            if message.is_empty() {
                ptr::null()
            } else {
                message.as_ptr()
            },
            message.len(),
            if embed { 1 } else { 0 },
            out.as_mut_ptr(),
            out.len(),
            &mut written,
        )
    })?;
    out.truncate(written);
    Ok(out)
}

pub fn rsg_validate(rsg: &[u8], message: &[u8], required_signer_hash: &[u8]) -> Result<()> {
    map_code(unsafe {
        ffi::rns_rsg_validate(
            if rsg.is_empty() {
                ptr::null()
            } else {
                rsg.as_ptr()
            },
            rsg.len(),
            if message.is_empty() {
                ptr::null()
            } else {
                message.as_ptr()
            },
            message.len(),
            if required_signer_hash.is_empty() {
                ptr::null()
            } else {
                required_signer_hash.as_ptr()
            },
            required_signer_hash.len(),
        )
    })
}

pub fn rsm_verify(rsm: &[u8], required_signer_hash: &[u8]) -> Result<Vec<u8>> {
    let mut needed = 0usize;
    let probe = unsafe {
        ffi::rns_rsm_verify(
            if rsm.is_empty() {
                ptr::null()
            } else {
                rsm.as_ptr()
            },
            rsm.len(),
            if required_signer_hash.is_empty() {
                ptr::null()
            } else {
                required_signer_hash.as_ptr()
            },
            required_signer_hash.len(),
            ptr::null_mut(),
            0,
            &mut needed,
        )
    };
    if probe != 0 && probe != 8 {
        map_code(probe)?;
    }
    if needed == 0 {
        return Ok(Vec::new());
    }
    let mut out = vec![0u8; needed];
    let mut written = 0usize;
    map_code(unsafe {
        ffi::rns_rsm_verify(
            if rsm.is_empty() {
                ptr::null()
            } else {
                rsm.as_ptr()
            },
            rsm.len(),
            if required_signer_hash.is_empty() {
                ptr::null()
            } else {
                required_signer_hash.as_ptr()
            },
            required_signer_hash.len(),
            out.as_mut_ptr(),
            out.len(),
            &mut written,
        )
    })?;
    out.truncate(written);
    Ok(out)
}
