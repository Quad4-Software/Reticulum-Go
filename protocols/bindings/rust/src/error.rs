// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CStr;

use crate::ffi;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Error {
    Ok = 0,
    InvalidArg = 1,
    InvalidHandle = 2,
    NotFound = 3,
    State = 4,
    Io = 5,
    Internal = 6,
    Timeout = 7,
    Truncated = 8,
}

pub type Result<T> = std::result::Result<T, Error>;

pub fn version() -> String {
    unsafe {
        let ptr = ffi::rrc_version();
        if ptr.is_null() {
            return String::new();
        }
        CStr::from_ptr(ptr).to_string_lossy().into_owned()
    }
}

pub fn last_error() -> String {
    let mut buf = vec![0u8; 512];
    let mut written = 0usize;
    let code = unsafe {
        ffi::rrc_last_error(buf.as_mut_ptr() as *mut i8, buf.len(), &mut written)
    };
    let msg = String::from_utf8_lossy(&buf[..written]).trim_end_matches('\0').to_string();
    if !msg.is_empty() {
        return msg;
    }
    if code == Error::Truncated as i32 {
        return "truncated".to_string();
    }
    String::new()
}

pub fn map_code(code: i32) -> Result<()> {
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
