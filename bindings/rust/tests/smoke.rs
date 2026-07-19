// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

use rns::{version, Identity, Node, API_VERSION, Error};

#[test]
fn version_matches_api() {
    assert_eq!(version(), API_VERSION);
}

#[test]
fn node_lifecycle() {
    let node = Node::create("").expect("node create");
    node.start().expect("start");
    assert!(matches!(node.event_poll(10), Err(Error::Timeout)));
    node.stop().expect("stop");
}

#[test]
fn identity_generate_hash_sign() {
    let id = Identity::generate().expect("generate");
    let hex = id.hash_hex().expect("hash");
    assert_eq!(hex.len(), 32);
    let bytes = id.hash_bytes().expect("hash bytes");
    assert_eq!(bytes.len(), 16);
    let msg = b"hello-rns";
    let sig = id.sign(msg).expect("sign");
    id.verify(msg, &sig).expect("verify");
    let pub_key = id.public_key().expect("pubkey");
    let only_pub = Identity::from_public_key(&pub_key).expect("from pubkey");
    only_pub.verify(msg, &sig).expect("verify via pubkey");
}
