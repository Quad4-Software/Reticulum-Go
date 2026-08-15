// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//! Idiomatic Rust bindings for the librns C ABI.
//!
//! Link against `bin/librns.so`. Keep `include/rns.h` as the ABI source of truth.

mod destination;
mod error;
mod event;
mod ffi;
mod identity;
mod interfaces;
mod link;
mod node;
mod path;
mod rsg;
mod util;

pub use destination::Destination;
pub use error::{last_error, map_code, version, Error, Result};
pub use event::{hash_eq, Event};
pub use ffi::{EventKind, InterfaceEntry, PathEntry, HASH_LEN};
pub use identity::Identity;
pub use interfaces::interfaces_list;
pub use link::{request_respond, request_respond_file, Link};
pub use node::{refresh_paths, Node};
pub use path::{path_known, path_request, path_table, PathInfo};
pub use rsg::{rsg_create, rsg_validate, rsm_verify};
pub use util::{hash_to_hex, hex_to_hash};

pub const API_VERSION: &str = "1.5";
