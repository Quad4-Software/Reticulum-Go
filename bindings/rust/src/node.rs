// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CString;

use crate::error::{map_code, Error, Result};
use crate::ffi::{self, HASH_LEN};
use crate::identity::Identity;

pub struct Node {
    handle: u64,
}

impl Node {
    pub fn create(config_path: &str) -> Result<Self> {
        let c = CString::new(config_path).map_err(|_| Error::InvalidArg)?;
        let handle = unsafe { ffi::rrc_node_create(c.as_ptr()) };
        if handle == 0 {
            return Err(Error::Internal);
        }
        Ok(Self { handle })
    }

    pub fn start(&self) -> Result<()> {
        map_code(unsafe { ffi::rrc_node_start(self.handle) })
    }

    pub fn stop(&self) -> Result<()> {
        map_code(unsafe { ffi::rrc_node_stop(self.handle) })
    }

    pub fn set_identity(&self, identity: &Identity) -> Result<()> {
        map_code(unsafe { ffi::rrc_node_set_identity(self.handle, identity.handle()) })
    }

    pub fn add_udp_interface(&self, name: &str, local_addr: &str, peer_addr: &str) -> Result<()> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let local_c = CString::new(local_addr).map_err(|_| Error::InvalidArg)?;
        let peer_c = CString::new(peer_addr).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_node_add_udp_interface(
                self.handle,
                name_c.as_ptr(),
                local_c.as_ptr(),
                peer_c.as_ptr(),
            )
        })
    }

    /// Attach an outbound TCP interface to host:port.
    pub fn add_tcp_client_interface(&self, name: &str, target: &str) -> Result<()> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let target_c = CString::new(target).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_node_add_tcp_client_interface(self.handle, name_c.as_ptr(), target_c.as_ptr())
        })
    }

    /// Attach an inbound TCP listener on host:port.
    pub fn add_tcp_server_interface(&self, name: &str, listen: &str) -> Result<()> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let listen_c = CString::new(listen).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_node_add_tcp_server_interface(self.handle, name_c.as_ptr(), listen_c.as_ptr())
        })
    }

    /// Attach an AutoInterface (UDP multicast LAN discovery).
    pub fn add_auto_interface(&self, name: &str) -> Result<()> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe { ffi::rrc_node_add_auto_interface(self.handle, name_c.as_ptr()) })
    }

    /// Attach a Backbone client dialing host:port.
    pub fn add_backbone_client_interface(&self, name: &str, target: &str) -> Result<()> {
        let name_c = CString::new(name).map_err(|_| Error::InvalidArg)?;
        let target_c = CString::new(target).map_err(|_| Error::InvalidArg)?;
        map_code(unsafe {
            ffi::rrc_node_add_backbone_client_interface(
                self.handle,
                name_c.as_ptr(),
                target_c.as_ptr(),
            )
        })
    }

    /// Number of interfaces currently attached.
    pub fn interface_count(&self) -> i32 {
        unsafe { ffi::rrc_node_interface_count(self.handle) }
    }

    /// Names of attached interfaces.
    pub fn interface_names(&self) -> Result<Vec<String>> {
        let mut buf = vec![0u8; 1024];
        let mut written = 0usize;
        map_code(unsafe {
            ffi::rrc_node_interface_names(
                self.handle,
                buf.as_mut_ptr() as *mut _,
                buf.len(),
                &mut written,
            )
        })?;
        let s = std::str::from_utf8(&buf[..written]).map_err(|_| Error::Internal)?;
        Ok(s.lines().map(String::from).collect())
    }

    pub fn has_path(&self, dest_hash: &[u8]) -> Result<bool> {
        if dest_hash.len() != HASH_LEN {
            return Err(Error::InvalidArg);
        }
        let mut out = 0i32;
        map_code(unsafe {
            ffi::rrc_node_has_path(
                self.handle,
                dest_hash.as_ptr(),
                dest_hash.len(),
                &mut out,
            )
        })?;
        Ok(out != 0)
    }

    pub fn handle(&self) -> u64 {
        self.handle
    }
}

impl Drop for Node {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                let _ = ffi::rrc_node_destroy(self.handle);
            }
            self.handle = 0;
        }
    }
}
