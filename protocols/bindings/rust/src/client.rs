// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;

use crate::error::{map_code, Error, Result};
use crate::event::{poll_client, Event};
use crate::ffi::{self, HASH_LEN};

pub const DEFAULT_TIMEOUT_MS: i32 = 45000;

pub struct Client {
    handle: u64,
}

impl Client {
    pub fn dial(
        node: u64,
        identity: u64,
        hub_hash: &[u8],
        nick: &str,
        name: &str,
        version: &str,
        timeout_ms: i32,
    ) -> Result<Self> {
        if hub_hash.len() != HASH_LEN {
            return Err(Error::InvalidArg);
        }
        let nick_c = CString::new(nick).map_err(|_| Error::InvalidArg)?;
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let ver_c = CString::new(version).map_err(|_| Error::InvalidArg)?;
        let handle = unsafe {
            ffi::rrc_client_dial(
                node,
                identity,
                hub_hash.as_ptr(),
                hub_hash.len(),
                nick_c.as_ptr(),
                name_c.as_ptr(),
                ver_c.as_ptr(),
                timeout_ms,
            )
        };
        if handle == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle })
    }

    pub fn join(&self, room: &str) -> Result<()> {
        let c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_client_join(self.handle, c.as_ptr()) })
    }

    pub fn part(&self, room: &str) -> Result<()> {
        let c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_client_part(self.handle, c.as_ptr()) })
    }

    pub fn send_msg(&self, room: &str, text: &str) -> Result<()> {
        let room_c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        let text_c = CString::new(text).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_client_send_msg(self.handle, room_c.as_ptr(), text_c.as_ptr())
        })
    }

    pub fn send_notice(&self, room: &str, text: &str) -> Result<()> {
        let room_c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        let text_c = CString::new(text).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_client_send_notice(self.handle, room_c.as_ptr(), text_c.as_ptr())
        })
    }

    pub fn send_action(&self, room: &str, text: &str) -> Result<()> {
        let room_c = CString::new(room).map_err(|_| Error::InvalidArg)?;
        let text_c = CString::new(text).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_client_send_action(self.handle, room_c.as_ptr(), text_c.as_ptr())
        })
    }

    pub fn ping(&self) -> Result<()> {
        map_code(unsafe { ffi::rrc_client_ping(self.handle) })
    }

    pub fn event_poll(&self, timeout_ms: i32) -> Result<Event> {
        poll_client(self.handle, timeout_ms)
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Client {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rrc_client_close(self.handle);
            }
            self.handle = 0;
        }
    }
}
