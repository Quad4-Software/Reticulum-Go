// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use crate::error::{Error, Result};
use crate::ffi::HASH_LEN;

pub fn hash_to_hex(hash: &[u8]) -> Result<String> {
    if hash.len() != HASH_LEN {
        return Err(Error::InvalidArg);
    }
    let mut out = String::with_capacity(HASH_LEN * 2);
    for b in hash {
        out.push_str(&format!("{:02x}", b));
    }
    Ok(out)
}

pub fn hex_to_hash(hex: &str) -> Result<[u8; HASH_LEN]> {
    if hex.len() != HASH_LEN * 2 {
        return Err(Error::InvalidArg);
    }
    let mut out = [0u8; HASH_LEN];
    for (i, chunk) in hex.as_bytes().chunks(2).enumerate() {
        if chunk.len() != 2 {
            return Err(Error::InvalidArg);
        }
        let hi = hex_digit(chunk[0])?;
        let lo = hex_digit(chunk[1])?;
        out[i] = (hi << 4) | lo;
    }
    Ok(out)
}

fn hex_digit(b: u8) -> Result<u8> {
    match b {
        b'0'..=b'9' => Ok(b - b'0'),
        b'a'..=b'f' => Ok(b - b'a' + 10),
        b'A'..=b'F' => Ok(b - b'A' + 10),
        _ => Err(Error::InvalidArg),
    }
}

pub fn cstr_field(buf: &[i8]) -> String {
    let bytes: Vec<u8> = buf
        .iter()
        .map(|b| *b as u8)
        .take_while(|b| *b != 0)
        .collect();
    String::from_utf8_lossy(&bytes).into_owned()
}
