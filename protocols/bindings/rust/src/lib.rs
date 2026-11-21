// SPDX-License-Identifier: LicenseRef-Reticulum

//! Idiomatic Rust bindings for the librrc C ABI.

mod buffers;
mod client;
mod envelope;
mod error;
mod event;
mod ffi;
mod hub;
mod identity;
mod node;
mod util;

pub use client::{Client, DEFAULT_TIMEOUT_MS};
pub use envelope::Envelope;
pub use error::{last_error, map_code, version, Error, Result};
pub use event::{Event, EventKind};
pub use ffi::{HASH_LEN, RRC_TYPE_MSG};
pub use hub::Hub;
pub use identity::Identity;
pub use node::Node;
pub use util::{hash_to_hex, hex_to_hash, normalize_room, sanitize_nick};

pub const API_VERSION: &str = "1.0";
