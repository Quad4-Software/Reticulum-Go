// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;

use crate::buffers::grow_string;
use crate::error::{Error, Result};
use crate::ffi;

pub fn normalize_room(name: &str) -> Result<String> {
    let input = CString::new(name).map_err(|_| Error::InvalidArg)?;
    grow_string(128, |buf, written| unsafe {
        ffi::rrc_normalize_room(input.as_ptr(), buf.as_mut_ptr(), buf.len(), written)
    })
}

pub fn sanitize_nick(nick: &str) -> Result<String> {
    let input = CString::new(nick).map_err(|_| Error::InvalidArg)?;
    grow_string(64, |buf, written| unsafe {
        ffi::rrc_sanitize_nick(input.as_ptr(), buf.as_mut_ptr(), buf.len(), written)
    })
}

pub fn hash_to_hex(data: &[u8]) -> String {
    data.iter().map(|b| format!("{b:02x}")).collect()
}

pub fn hex_to_hash(text: &str) -> Result<Vec<u8>> {
    if text.len() % 2 != 0 {
        return Err(Error::InvalidArg);
    }
    let mut out = Vec::with_capacity(text.len() / 2);
    let bytes = text.as_bytes();
    for i in (0..bytes.len()).step_by(2) {
        let hi = from_hex_digit(bytes[i])?;
        let lo = from_hex_digit(bytes[i + 1])?;
        out.push((hi << 4) | lo);
    }
    Ok(out)
}

fn from_hex_digit(b: u8) -> Result<u8> {
    match b {
        b'0'..=b'9' => Ok(b - b'0'),
        b'a'..=b'f' => Ok(b - b'a' + 10),
        b'A'..=b'F' => Ok(b - b'A' + 10),
        _ => Err(Error::InvalidArg),
    }
}
