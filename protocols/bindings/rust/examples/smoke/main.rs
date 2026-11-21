// SPDX-License-Identifier: LicenseRef-Reticulum

use std::ffi::CStr;
use std::os::raw::c_char;

#[link(name = "rrc")]
unsafe extern "C" {
    fn rrc_version() -> *const c_char;
    fn rrc_node_create(config_path: *const c_char) -> u64;
    fn rrc_node_start(node: u64) -> i32;
    fn rrc_node_stop(node: u64) -> i32;
    fn rrc_node_destroy(node: u64) -> i32;
}

fn main() {
    unsafe {
        let ver = CStr::from_ptr(rrc_version()).to_string_lossy();
        if ver != "1.0" {
            eprintln!("unexpected version: {ver}");
            std::process::exit(1);
        }
        let node = rrc_node_create(b"\0".as_ptr() as *const c_char);
        if node == 0 {
            eprintln!("node create failed");
            std::process::exit(1);
        }
        if rrc_node_start(node) != 0 {
            eprintln!("node start failed");
            std::process::exit(1);
        }
        if rrc_node_stop(node) != 0 {
            eprintln!("node stop failed");
            std::process::exit(1);
        }
        rrc_node_destroy(node);
    }
    println!("rust-smoke ok");
}
