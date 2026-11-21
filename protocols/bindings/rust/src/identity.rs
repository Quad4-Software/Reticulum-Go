// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;
use std::path::Path;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};

pub struct Identity {
    handle: u64,
}

impl Identity {
    pub fn generate() -> Result<Self> {
        let handle = unsafe { ffi::rrc_identity_generate() };
        if handle == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle })
    }

    pub fn load(path: impl AsRef<Path>) -> Result<Self> {
        let c = CString::new(path.as_ref().to_string_lossy().as_bytes())
            .map_err(|_| Error::InvalidArg)?;
        let handle = unsafe { ffi::rrc_identity_load(c.as_ptr()) };
        if handle == 0 {
            return Err(Error::Io);
        }
        Ok(Self { handle })
    }

    pub fn save(&self, path: impl AsRef<Path>) -> Result<()> {
        let c = CString::new(path.as_ref().to_string_lossy().as_bytes())
            .map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_identity_save(self.handle, c.as_ptr()) })
    }

    pub fn hash_bytes(&self) -> Result<Vec<u8>> {
        let mut buf = vec![0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rrc_identity_hash(self.handle, buf.as_mut_ptr(), buf.len(), &mut written)
        })?;
        buf.truncate(written);
        Ok(buf)
    }

    pub fn seed_destination(&self, dest_hash: &[u8]) -> Result<()> {
        if dest_hash.len() != HASH_LEN {
            return Err(Error::InvalidArg);
        }
        map_code(unsafe {
            ffi::rrc_identity_seed_destination(self.handle, dest_hash.as_ptr(), dest_hash.len())
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
                let _ = ffi::rrc_identity_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
