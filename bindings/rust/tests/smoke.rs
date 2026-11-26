// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

use rns::{
    destination_encrypt, rsg_sign_file, rsg_verify_file, set_event_callback, version, Destination,
    Error, Identity, Link, Node, API_VERSION,
};

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

#[test]
fn added_abi_surface_smoke() {
    let node = Node::create("").expect("node create");
    assert_eq!(node.reload_config(), Err(Error::InvalidArg));
    let config_path = std::env::temp_dir().join("rns-rust-reload.conf");
    std::fs::write(
        &config_path,
        "[reticulum]\nenable_transport = no\nin_memory_storage = yes\n",
    )
    .expect("write config");
    let configured =
        Node::create(config_path.to_str().expect("config path utf8")).expect("configured node");
    assert_eq!(configured.reload_config(), Ok(()));
    let _ = std::fs::remove_file(&config_path);
    let unknown = [0u8; 16];
    assert_eq!(destination_encrypt(&unknown, b"x"), Err(Error::NotFound));
    assert_eq!(node.packet_send(&unknown, b"x"), Err(Error::State));
    assert!(matches!(
        Link::from_id(&node, &unknown),
        Err(Error::NotFound)
    ));
    let id = Identity::generate().expect("generate");
    let dest = Destination::create(&node, Some(&id), "rusttest", &[], false).expect("destination");
    assert_eq!(dest.set_proof_strategy(0), Ok(()));
    assert_eq!(dest.set_proof_strategy(255), Err(Error::InvalidArg));
    set_event_callback(&node, Some(|_| {})).expect("set callback");
    set_event_callback::<fn(rns::Event)>(&node, None).expect("clear callback");
    let signed_path = std::env::temp_dir().join("rns-rust-signed.txt");
    std::fs::write(&signed_path, b"signed").expect("write signed file");
    let sig =
        rsg_sign_file(&id, signed_path.to_str().expect("signed path utf8")).expect("sign file");
    rsg_verify_file(&sig, signed_path.to_str().expect("signed path utf8"), &[])
        .expect("verify file");
    std::fs::write(&signed_path, b"tampered").expect("tamper file");
    assert!(rsg_verify_file(&sig, signed_path.to_str().expect("signed path utf8"), &[]).is_err());
    let _ = std::fs::remove_file(&signed_path);
}
