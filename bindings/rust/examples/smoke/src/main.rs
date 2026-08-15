// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use rns::{last_error, version, Error, Node, API_VERSION};

fn main() {
    let ver = version();
    if ver != API_VERSION {
        eprintln!("unexpected version: {ver}");
        std::process::exit(1);
    }

    let node = match Node::create("") {
        Ok(node) => node,
        Err(_) => {
            eprintln!("rns::Node::create failed: {}", last_error());
            std::process::exit(1);
        }
    };

    if node.start().is_err() {
        eprintln!("node.start failed: {}", last_error());
        std::process::exit(1);
    }

    if !matches!(node.event_poll(10), Err(Error::Timeout)) {
        eprintln!("expected timeout poll on idle node");
        let _ = node.stop();
        std::process::exit(1);
    }

    if node.stop().is_err() {
        eprintln!("teardown failed");
        std::process::exit(1);
    }

    println!("rust-smoke ok");
}
