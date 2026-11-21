// SPDX-License-Identifier: LicenseRef-Reticulum

use std::env;
use std::path::PathBuf;

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let default_bin = manifest.join("../../bin");
    let lib_dir = env::var("RRC_LIB_DIR").unwrap_or_else(|_| default_bin.display().to_string());
    println!("cargo:rustc-link-search=native={lib_dir}");
    println!("cargo:rustc-link-lib=dylib=rrc");
    println!("cargo:rerun-if-env-changed=RRC_LIB_DIR");
}
