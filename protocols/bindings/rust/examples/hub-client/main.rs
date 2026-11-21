// SPDX-License-Identifier: LicenseRef-Reticulum

use std::thread;
use std::time::{Duration, Instant};

use rrc::{Client, EventKind, Hub, Identity, Node};

const HUB_LOCAL: &str = "127.0.0.1:42550";
const HUB_PEER: &str = "127.0.0.1:42551";
const CLI_LOCAL: &str = "127.0.0.1:42551";
const CLI_PEER: &str = "127.0.0.1:42550";

fn main() {
    if let Err(err) = run() {
        eprintln!("{err:?}");
        std::process::exit(1);
    }
    println!("rust-hub-client ok");
}

fn run() -> rrc::Result<()> {
    let hub_node = Node::create("")?;
    let cli_node = Node::create("")?;
    hub_node.add_udp_interface("H1", HUB_LOCAL, HUB_PEER)?;
    cli_node.add_udp_interface("C1", CLI_LOCAL, CLI_PEER)?;

    let id_h = Identity::generate()?;
    let id_c = Identity::generate()?;
    hub_node.set_identity(&id_h)?;
    cli_node.set_identity(&id_c)?;
    hub_node.start()?;
    cli_node.start()?;

    let hub = Hub::create(hub_node.handle(), id_h.handle(), "rust-hub", "1.0")?;
    hub.start()?;
    hub.announce()?;
    let hub_hash = hub.hash_bytes()?;
    id_h.seed_destination(&hub_hash)?;

    let deadline = Instant::now() + Duration::from_secs(15);
    while Instant::now() < deadline {
        if cli_node.has_path(&hub_hash)? {
            break;
        }
        thread::sleep(Duration::from_millis(50));
    }
    if Instant::now() >= deadline {
        return Err(rrc::Error::Timeout);
    }

    let client = Client::dial(
        cli_node.handle(),
        id_c.handle(),
        &hub_hash,
        "alice",
        "rust-client",
        "1.0",
        15000,
    )?;
    client.join("#lobby")?;

    let joined_deadline = Instant::now() + Duration::from_secs(10);
    let mut joined = false;
    while Instant::now() < joined_deadline {
        match client.event_poll(500) {
            Ok(ev) if ev.kind == EventKind::Joined => {
                joined = true;
                break;
            }
            Err(rrc::Error::Timeout) => {}
            Err(err) => return Err(err),
            Ok(_) => {}
        }
    }
    if !joined {
        return Err(rrc::Error::Timeout);
    }

    let want = "hello from rust hub-client";
    client.send_msg("#lobby", want)?;

    let msg_deadline = Instant::now() + Duration::from_secs(10);
    while Instant::now() < msg_deadline {
        match hub.event_poll(500) {
            Ok(ev) if ev.kind == EventKind::Msg && ev.body == want => return Ok(()),
            Err(rrc::Error::Timeout) => {}
            Err(err) => return Err(err),
            Ok(_) => {}
        }
    }
    Err(rrc::Error::Timeout)
}
