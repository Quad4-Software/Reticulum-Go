// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;

use crate::buffers::{grow_bytes, grow_string};
use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};

pub struct Envelope {
    handle: u64,
}

impl Envelope {
    pub fn create(msg_type: u64, sender: &[u8]) -> Result<Self> {
        if sender.len() != HASH_LEN {
            return Err(Error::InvalidArg);
        }
        let handle = unsafe { ffi::rrc_envelope_create(msg_type, sender.as_ptr(), sender.len()) };
        if handle == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle })
    }

    pub fn unmarshal(data: &[u8]) -> Result<Self> {
        let handle = unsafe { ffi::rrc_envelope_unmarshal(data.as_ptr(), data.len()) };
        if handle == 0 {
            return Err(Error::InvalidArg);
        }
        Ok(Self { handle })
    }

    pub fn set_room(&self, room: &str) -> Result<()> {
        let c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_envelope_set_room(self.handle, c.as_ptr()) })
    }

    pub fn set_nick(&self, nick: &str) -> Result<()> {
        let c = CString::new(nick).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_envelope_set_nick(self.handle, c.as_ptr()) })
    }

    pub fn set_body_text(&self, text: &str) -> Result<()> {
        let c = CString::new(text).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_envelope_set_body_text(self.handle, c.as_ptr()) })
    }

    pub fn set_destination(&self, dest: &[u8]) -> Result<()> {
        if dest.len() != HASH_LEN {
            return Err(Error::InvalidArg);
        }
        map_code(unsafe {
            ffi::rrc_envelope_set_destination(self.handle, dest.as_ptr(), dest.len())
        })
    }

    pub fn msg_type(&self) -> Result<u64> {
        let mut out = 0u64;
        map_code(unsafe { ffi::rrc_envelope_get_type(self.handle, &mut out) })?;
        Ok(out)
    }

    pub fn sender(&self) -> Result<Vec<u8>> {
        let mut buf = vec![0u8; HASH_LEN];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rrc_envelope_get_sender(self.handle, buf.as_mut_ptr(), buf.len(), &mut written)
        })?;
        buf.truncate(written);
        Ok(buf)
    }

    pub fn room(&self) -> Result<String> {
        grow_string(128, |buf, written| unsafe {
            ffi::rrc_envelope_get_room(self.handle, buf.as_mut_ptr(), buf.len(), written)
        })
    }

    pub fn nick(&self) -> Result<String> {
        grow_string(64, |buf, written| unsafe {
            ffi::rrc_envelope_get_nick(self.handle, buf.as_mut_ptr(), buf.len(), written)
        })
    }

    pub fn body_text(&self) -> Result<String> {
        grow_string(1024, |buf, written| unsafe {
            ffi::rrc_envelope_get_body_text(self.handle, buf.as_mut_ptr(), buf.len(), written)
        })
    }

    pub fn marshal(&self) -> Result<Vec<u8>> {
        grow_bytes(65536, |buf, written| unsafe {
            ffi::rrc_envelope_marshal(self.handle, buf.as_mut_ptr(), buf.len(), written)
        })
    }
}

impl Drop for Envelope {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rrc_envelope_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
