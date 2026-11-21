// SPDX-License-Identifier: LicenseRef-Reticulum

use rrc::{version, Envelope, Identity, Node, API_VERSION, HASH_LEN, RRC_TYPE_MSG};

#[test]
fn version_matches_api() {
    assert_eq!(version(), API_VERSION);
}

#[test]
fn envelope_roundtrip() {
    let sender: Vec<u8> = (0..HASH_LEN).map(|i| i as u8).collect();
    let env = Envelope::create(RRC_TYPE_MSG, &sender).expect("create");
    env.set_room("lobby").expect("room");
    env.set_body_text("hello").expect("body");
    let data = env.marshal().expect("marshal");

    let got = Envelope::unmarshal(&data).expect("unmarshal");
    assert_eq!(got.body_text().expect("text"), "hello");
}

#[test]
fn node_lifecycle() {
    let node = Node::create("").expect("node");
    node.start().expect("start");
    node.stop().expect("stop");
}

#[test]
fn identity_hash() {
    let id = Identity::generate().expect("generate");
    assert_eq!(id.hash_bytes().expect("hash").len(), HASH_LEN);
}

#[test]
fn normalize_sanitize() {
    assert_eq!(rrc::normalize_room("  #Lobby ").expect("room"), "#lobby");
    assert_eq!(rrc::sanitize_nick(" alice ").expect("nick"), "alice");
}
