// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::ffi::CString;
use std::ptr;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};

pub struct Identity {
    handle: u64,
}

impl Identity {
    pub fn generate() -> Result<Self> {
        let h = unsafe { ffi::rns_identity_generate() };
        if h == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle: h })
    }

    pub fn load(path: &str) -> Result<Self> {
        if path.is_empty() {
            return Err(Error::InvalidArg);
        }
        let path = CString::new(path).map_err(|_| Error::InvalidArg)?;
        let h = unsafe { ffi::rns_identity_load(path.as_ptr()) };
        if h == 0 {
            return Err(Error::Io);
        }
        Ok(Self { handle: h })
    }

    pub fn save(&self, path: &str) -> Result<()> {
        if path.is_empty() {
            return Err(Error::InvalidArg);
        }
        let path = CString::new(path).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rns_identity_save(self.handle, path.as_ptr()) })
    }

    pub fn hash_hex(&self) -> Result<String> {
        let mut buf = [0i8; 64];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_identity_hash(self.handle, buf.as_mut_ptr(), buf.len(), &mut written)
        })?;
        let bytes = unsafe { std::slice::from_raw_parts(buf.as_ptr() as *const u8, written) };
        Ok(String::from_utf8_lossy(bytes).into_owned())
    }

    pub fn hash_bytes(&self) -> Result<[u8; HASH_LEN]> {
        let mut out = [0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_identity_hash_bytes(self.handle, out.as_mut_ptr(), out.len(), &mut written)
        })?;
        if written != HASH_LEN {
            return Err(Error::Truncated);
        }
        Ok(out)
    }

    pub fn public_key(&self) -> Result<Vec<u8>> {
        let mut buf = [0u8; 64];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_identity_public_key(self.handle, buf.as_mut_ptr(), buf.len(), &mut written)
        })?;
        Ok(buf[..written].to_vec())
    }

    pub fn from_public_key(pub_key: &[u8]) -> Result<Self> {
        if pub_key.is_empty() {
            return Err(Error::InvalidArg);
        }
        let h = unsafe { ffi::rns_identity_from_public_key(pub_key.as_ptr(), pub_key.len()) };
        if h == 0 {
            return Err(Error::InvalidArg);
        }
        Ok(Self { handle: h })
    }

    pub fn sign(&self, data: &[u8]) -> Result<Vec<u8>> {
        let mut buf = [0u8; 64];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rns_identity_sign(
                self.handle,
                if data.is_empty() {
                    ptr::null()
                } else {
                    data.as_ptr()
                },
                data.len(),
                buf.as_mut_ptr(),
                buf.len(),
                &mut written,
            )
        })?;
        Ok(buf[..written].to_vec())
    }

    pub fn verify(&self, data: &[u8], signature: &[u8]) -> Result<()> {
        map_code(unsafe {
            ffi::rns_identity_verify(
                self.handle,
                if data.is_empty() {
                    ptr::null()
                } else {
                    data.as_ptr()
                },
                data.len(),
                if signature.is_empty() {
                    ptr::null()
                } else {
                    signature.as_ptr()
                },
                signature.len(),
            )
        })
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Identity {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rns_identity_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
