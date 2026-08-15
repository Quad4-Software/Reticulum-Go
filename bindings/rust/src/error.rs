// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ffi::CStr;
use std::os::raw::c_int;

use crate::ffi;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Error {
    InvalidArg,
    InvalidHandle,
    NotFound,
    State,
    Io,
    Internal,
    Timeout,
    Truncated,
}

pub type Result<T> = std::result::Result<T, Error>;

pub fn map_code(code: c_int) -> Result<()> {
    match code {
        0 => Ok(()),
        1 => Err(Error::InvalidArg),
        2 => Err(Error::InvalidHandle),
        3 => Err(Error::NotFound),
        4 => Err(Error::State),
        5 => Err(Error::Io),
        6 => Err(Error::Internal),
        7 => Err(Error::Timeout),
        8 => Err(Error::Truncated),
        _ => Err(Error::Internal),
    }
}

pub fn version() -> String {
    unsafe {
        let ptr = ffi::rns_version();
        if ptr.is_null() {
            return String::new();
        }
        CStr::from_ptr(ptr).to_string_lossy().into_owned()
    }
}

pub fn last_error() -> String {
    let mut buf = [0i8; 512];
    let mut written = 0usize;
    unsafe {
        if ffi::rns_last_error(buf.as_mut_ptr(), buf.len(), &mut written) != 0 {
            return String::new();
        }
        let n = written.min(buf.len());
        CStr::from_ptr(buf.as_ptr())
            .to_string_lossy()
            .chars()
            .take(n)
            .collect()
    }
}
