// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over the Rust librns bindings.
// Usage: rns-pageserver -c config [-i identity] [-a announce_sec] [-p page_file] [-P request_path]

use std::env;
use std::fs;
use std::thread;
use std::time::{Duration, Instant};

use rns::{
    hash_to_hex, last_error, request_respond, request_respond_file, version, Destination, Error,
    Event, EventKind, Identity, Node, API_VERSION,
};

const DEFAULT_ANNOUNCE_SEC: u64 = 900;
const DEFAULT_PAGE_PATH: &str = "/page/index.mu";
const DEFAULT_FILE_PATH: &str = "/file/test.txt";
const DEFAULT_PAGE_FILE: &str = "pages/index.mu";
const DEFAULT_FILE_FILE: &str = "files/test.txt";
const DEFAULT_IDENTITY_PATH: &str = "identity";
const REQ_DATA_CAP: usize = 64 * 1024;

const FALLBACK_PAGE: &str = "> Rust pageserver\n\n\
librns via Reticulum-Go\n\n\
Fallback page (file not found).\n\n\
`[Home`:/page/index.mu]\n\
`[Download Test File`:/file/test.txt]`_`f\n\n\
---\n";

const FALLBACK_FILE: &str = "Test file from Reticulum-Go node!\n";

fn usage(argv0: &str) {
    eprintln!(
        "Usage: {argv0} -c config [-i identity] [-a announce_sec] [-p page_file] [-f file] [-P request_path]\n\n\
         Serve a NomadNet-compatible /page/ handler over librns.\n\
         Destination: nomadnetwork.node\n\
         Announce app_data name: librns-rust-pageserver\n\n\
         Options:\n\
           -c path   Reticulum config file (required)\n\
           -i path   Persistent identity file (default {DEFAULT_IDENTITY_PATH})\n\
           -a sec    Announce interval seconds (default {DEFAULT_ANNOUNCE_SEC}, 0 = once)\n\
           -p file   Micron page file to serve (default {DEFAULT_PAGE_FILE})\n\
           -f file   Download file to serve at /file/test.txt (default {DEFAULT_FILE_FILE})\n\
           -P path   Request path to register (default {DEFAULT_PAGE_PATH})"
    );
}

fn print_last_error(what: &str) {
    let msg = last_error();
    if msg.is_empty() {
        eprintln!("{what}");
    } else {
        eprintln!("{what}: {msg}");
    }
}

fn load_bytes(path: &str, fallback: &str) -> Vec<u8> {
    match fs::read(path) {
        Ok(data) => data,
        Err(_) => {
            eprintln!("warning: could not read {path}, using built-in content");
            fallback.as_bytes().to_vec()
        }
    }
}

fn load_or_create_identity(path: &str) -> Result<Identity, ()> {
    match Identity::load(path) {
        Ok(id) => {
            eprintln!("loaded identity from {path}");
            Ok(id)
        }
        Err(_) => {
            let id = Identity::generate().map_err(|_| ())?;
            id.save(path).map_err(|_| ())?;
            eprintln!("created and saved identity to {path}");
            Ok(id)
        }
    }
}

fn run(
    config_path: &str,
    identity_path: &str,
    page_file: &str,
    file_file: &str,
    request_path: &str,
    file_path: &str,
    announce_sec: u64,
) -> i32 {
    let ver = version();
    if ver != API_VERSION {
        eprintln!("librns version mismatch: got {ver} want {API_VERSION}");
        return 1;
    }

    let page_body = load_bytes(page_file, FALLBACK_PAGE);
    let file_body = load_bytes(file_file, FALLBACK_FILE);

    let node = match Node::create(config_path) {
        Ok(v) => v,
        Err(_) => {
            print_last_error("Node::create failed");
            return 1;
        }
    };
    let identity = match load_or_create_identity(identity_path) {
        Ok(v) => v,
        Err(_) => {
            print_last_error("identity load/create failed");
            return 1;
        }
    };
    if node.set_identity(&identity).is_err() {
        print_last_error("node.set_identity failed");
        return 1;
    }
    if node.start().is_err() {
        print_last_error("node.start failed");
        return 1;
    }

    let dest = match Destination::create(&node, None, "nomadnetwork", &["node"], true) {
        Ok(v) => v,
        Err(_) => {
            print_last_error("Destination::create failed");
            return 1;
        }
    };
    if dest.register_request_handler(request_path).is_err() {
        print_last_error("register_request_handler failed");
        return 1;
    }
    if dest.register_request_handler(file_path).is_err() {
        print_last_error("register_request_handler file failed");
        return 1;
    }

    let dest_hash = match dest.hash() {
        Ok(v) => v,
        Err(_) => {
            print_last_error("destination.hash failed");
            return 1;
        }
    };
    let dest_hex = match hash_to_hex(&dest_hash) {
        Ok(v) => v,
        Err(_) => {
            eprintln!("failed to encode destination hash");
            return 1;
        }
    };

    println!("DEST_HASH={dest_hex}");
    println!("REQUEST_PATH={request_path}");
    println!("FILE_PATH={file_path}");
    eprintln!("librns {ver} pageserver listening as nomadnetwork.node");
    eprintln!("announce name=librns-rust-pageserver interval={announce_sec}s");
    eprintln!("serving {} bytes from {page_file}", page_body.len());
    eprintln!(
        "serving {} bytes from {file_file} as {file_path}",
        file_body.len()
    );

    let app_data = b"librns-rust-pageserver";
    if dest.announce(app_data).is_err() {
        print_last_error("destination.announce failed");
    } else {
        eprintln!("announce sent");
    }

    let mut req_buf = vec![0u8; REQ_DATA_CAP];
    let announce_every = Duration::from_secs(announce_sec);
    let mut last_announce = Instant::now();

    loop {
        if announce_sec > 0 && last_announce.elapsed() >= announce_every {
            if dest.announce(app_data).is_ok() {
                eprintln!("announce refreshed");
            }
            last_announce = Instant::now();
        }

        match Event::poll(&node, 200, &mut req_buf) {
            Ok(ev) => match ev.kind() {
                EventKind::LinkEstablished => eprintln!("inbound link established"),
                EventKind::LinkClosed => eprintln!("link closed"),
                EventKind::RequestIncoming => {
                    let path = ev.path();
                    eprintln!("request incoming path={path}");
                    let req_id = ev.request_id();
                    if path == request_path {
                        if request_respond(&node, req_id, &page_body).is_err() {
                            print_last_error("request_respond failed");
                        } else {
                            eprintln!("served {request_path} ({} bytes)", page_body.len());
                        }
                    } else if path == file_path {
                        if request_respond_file(&node, req_id, "test.txt", &file_body).is_err() {
                            print_last_error("request_respond_file failed");
                        } else {
                            eprintln!("served {file_path} ({} bytes)", file_body.len());
                        }
                    } else {
                        let msg = b"page not found\n";
                        if request_respond(&node, req_id, msg).is_err() {
                            print_last_error("request_respond failed");
                        }
                    }
                }
                _ => {}
            },
            Err(Error::Timeout) => {}
            Err(_) => {
                print_last_error("Event::poll failed");
                return 1;
            }
        }

        thread::sleep(Duration::from_millis(1));
    }
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let argv0 = args.first().map(String::as_str).unwrap_or("rns-pageserver");

    let mut config_path = String::new();
    let mut identity_path = DEFAULT_IDENTITY_PATH.to_string();
    let mut page_file = DEFAULT_PAGE_FILE.to_string();
    let mut file_file = DEFAULT_FILE_FILE.to_string();
    let mut request_path = DEFAULT_PAGE_PATH.to_string();
    let file_path = DEFAULT_FILE_PATH.to_string();
    let mut announce_sec = DEFAULT_ANNOUNCE_SEC;

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "-c" if i + 1 < args.len() => {
                i += 1;
                config_path = args[i].clone();
            }
            "-i" if i + 1 < args.len() => {
                i += 1;
                identity_path = args[i].clone();
            }
            "-a" if i + 1 < args.len() => {
                i += 1;
                announce_sec = args[i].parse().unwrap_or(0);
            }
            "-p" if i + 1 < args.len() => {
                i += 1;
                page_file = args[i].clone();
            }
            "-f" if i + 1 < args.len() => {
                i += 1;
                file_file = args[i].clone();
            }
            "-P" if i + 1 < args.len() => {
                i += 1;
                request_path = args[i].clone();
            }
            "-h" | "--help" => {
                usage(argv0);
                return;
            }
            arg => {
                eprintln!("unknown option: {arg}");
                usage(argv0);
                std::process::exit(1);
            }
        }
        i += 1;
    }

    if config_path.is_empty() {
        usage(argv0);
        std::process::exit(1);
    }

    std::process::exit(run(
        &config_path,
        &identity_path,
        &page_file,
        &file_file,
        &request_path,
        &file_path,
        announce_sec,
    ));
}
