// SPDX-License-Identifier: LicenseRef-Reticulum

use crate::error::{map_code, Error, Result};

const MAX_BUFFER: usize = 16 * 1024 * 1024;

pub(crate) fn grow_bytes<F>(initial: usize, mut fill: F) -> Result<Vec<u8>>
where
    F: FnMut(&mut [u8], &mut usize) -> i32,
{
    let mut cap = initial.max(64);
    loop {
        let mut buf = vec![0u8; cap];
        let mut written = 0usize;
        let code = fill(&mut buf, &mut written);
        if code == Error::Truncated as i32 {
            cap = cap.saturating_mul(2);
            if cap > MAX_BUFFER {
                return Err(Error::Truncated);
            }
            continue;
        }
        map_code(code)?;
        buf.truncate(written);
        return Ok(buf);
    }
}

pub(crate) fn grow_string<F>(initial: usize, mut fill: F) -> Result<String>
where
    F: FnMut(&mut [i8], &mut usize) -> i32,
{
    let mut cap = initial.max(64);
    loop {
        let mut buf = vec![0i8; cap];
        let mut written = 0usize;
        let code = fill(&mut buf, &mut written);
        if code == Error::Truncated as i32 {
            cap = cap.saturating_mul(2);
            if cap > MAX_BUFFER {
                return Err(Error::Truncated);
            }
            continue;
        }
        map_code(code)?;
        let bytes: Vec<u8> = buf[..written].iter().map(|&c| c as u8).collect();
        return Ok(String::from_utf8_lossy(&bytes)
            .trim_end_matches('\0')
            .to_string());
    }
}
