// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;

use crate::error::{map_code, Error, Result};
use crate::event::{poll_hub, Event};
use crate::ffi::{self, HASH_LEN};

pub struct Hub {
    handle: u64,
}

impl Hub {
    pub fn create(node: u64, identity: u64, name: &str, version: &str) -> Result<Self> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let ver_c = CString::new(version).map_err(|_| Error::InvalidArg)?;
        let handle = unsafe {
            ffi::rrc_hub_create(node, identity, name_c.as_ptr(), ver_c.as_ptr())
        };
        if handle == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle })
    }

    pub fn start(&self) -> Result<()> {
        map_code(unsafe { ffi::rrc_hub_start(self.handle) })
    }

    pub fn announce(&self) -> Result<()> {
        map_code(unsafe { ffi::rrc_hub_announce(self.handle) })
    }

    pub fn hash_bytes(&self) -> Result<Vec<u8>> {
        let mut buf = vec![0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rrc_hub_hash(self.handle, buf.as_mut_ptr(), buf.len(), &mut written)
        })?;
        buf.truncate(written);
        Ok(buf)
    }

    pub fn peer_count(&self) -> Result<usize> {
        let mut count = 0usize;
        map_code(unsafe { ffi::rrc_hub_peer_count(self.handle, &mut count) })?;
        Ok(count)
    }

    pub fn event_poll(&self, timeout_ms: i32) -> Result<Event> {
        poll_hub(self.handle, timeout_ms)
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Hub {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rrc_hub_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
