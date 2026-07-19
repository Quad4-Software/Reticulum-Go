// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use std::env;
use std::path::PathBuf;

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let default_bin = manifest.join("../../bin");
    let lib_dir = env::var("RNS_LIB_DIR").unwrap_or_else(|_| default_bin.display().to_string());
    println!("cargo:rustc-link-search=native={lib_dir}");
    println!("cargo:rustc-link-lib=dylib=rns");
    println!("cargo:rerun-if-env-changed=RNS_LIB_DIR");
}
