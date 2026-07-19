// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over the Rust librns bindings.
// Usage: rns-page-fetch [-c config] [-t timeout_sec] <dest_hash>:<page_path>

use std::env;
use std::io::{self, Write};
use std::time::{Duration, Instant};

use rns::{
    hash_eq, hash_to_hex, hex_to_hash, last_error, path_known, path_request, version, Error, Event,
    EventKind, Identity, Link, Node, API_VERSION,
};

const PAGE_BUF_CAP: usize = 512 * 1024;
const DEFAULT_TIMEOUT_SEC: u64 = 60;
const PATH_RETRY: Duration = Duration::from_secs(2);

fn usage(argv0: &str) {
    eprintln!(
        "Usage: {argv0} [-c config] [-t timeout_sec] <dest_hash>:<page_path>\n\n\
         Fetch a NomadNet / pageserver page over librns (Rust bindings).\n\n\
         Options:\n\
           -c path   Reticulum config file (required for network interfaces)\n\
           -t sec    Overall timeout in seconds (default {DEFAULT_TIMEOUT_SEC})\n\n\
         Example:\n\
           {argv0} -c config.example 92798ea245a0afcfa559348e42d628c6:/page/index.mu"
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

fn parse_target(target: &str) -> Option<([u8; 16], String)> {
    let (hex, path) = target.split_once(':')?;
    if hex.is_empty() || path.is_empty() {
        return None;
    }
    let hash = hex_to_hash(hex).ok()?;
    Some((hash, path.to_string()))
}

fn run(config_path: &str, target: &str, timeout_sec: u64) -> i32 {
    let ver = version();
    if ver != API_VERSION {
        eprintln!("librns version mismatch: got {ver} want {API_VERSION}");
        return 1;
    }

    let (dest_hash, page_path) = match parse_target(target) {
        Some(v) => v,
        None => {
            eprintln!("target must be <32-hex-dest>:<page_path>");
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

    let node = match Node::create(config_path) {
        Ok(v) => v,
        Err(_) => {
            print_last_error("Node::create failed");
            return 1;
        }
    };
    let identity = match Identity::generate() {
        Ok(v) => v,
        Err(_) => {
            print_last_error("Identity::generate failed");
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

    println!("librns {ver} fetching {page_path} from {dest_hex}");

    let mut page_buf = vec![0u8; PAGE_BUF_CAP];
    let deadline = Instant::now() + Duration::from_secs(timeout_sec);
    let mut last_path_req = Instant::now() - PATH_RETRY;
    let mut need_path_req = true;
    let mut saw_announce = false;
    let mut link: Option<Link> = None;

    while Instant::now() < deadline && link.is_none() {
        let now = Instant::now();
        if need_path_req || now.duration_since(last_path_req) >= PATH_RETRY {
            if path_request(&node, &dest_hash).is_err() {
                print_last_error("path_request failed");
            }
            last_path_req = now;
            need_path_req = false;
            if path_known(&node, &dest_hash) {
                eprintln!("path known, waiting for destination identity announce");
            } else {
                eprintln!("requesting path to {dest_hex}");
            }
        }

        match Event::poll(&node, 200, &mut page_buf) {
            Ok(ev) => {
                if ev.kind() == EventKind::Announce && hash_eq(ev.destination_hash(), &dest_hash) {
                    saw_announce = true;
                    eprintln!("announce for target (hops={})", ev.hops());
                    match Link::open(&node, &dest_hash) {
                        Ok(opened) => link = Some(opened),
                        Err(_) => print_last_error("Link::open after announce"),
                    }
                } else if ev.kind() == EventKind::LinkFailed {
                    eprintln!("link failed while opening: {}", ev.error_message());
                }
            }
            Err(Error::Timeout) => {
                if saw_announce || path_known(&node, &dest_hash) {
                    if let Ok(opened) = Link::open(&node, &dest_hash) {
                        link = Some(opened);
                    }
                }
            }
            Err(_) => {
                print_last_error("Event::poll failed");
                return 1;
            }
        }
    }

    let link = match link {
        Some(v) => v,
        None => {
            eprintln!("timed out before link open");
            return 1;
        }
    };

    let mut established = false;
    while Instant::now() < deadline && !established {
        match Event::poll(&node, 500, &mut page_buf) {
            Ok(ev) => match ev.kind() {
                EventKind::LinkEstablished => {
                    established = true;
                    eprintln!("link established");
                }
                EventKind::LinkFailed => {
                    eprintln!("link establishment failed: {}", ev.error_message());
                    return 1;
                }
                EventKind::LinkClosed => {
                    eprintln!("link closed before establish");
                    return 1;
                }
                _ => {}
            },
            Err(Error::Timeout) => {}
            Err(_) => {
                print_last_error("Event::poll failed");
                return 1;
            }
        }
    }

    if !established {
        eprintln!("timed out waiting for link establishment");
        return 1;
    }

    let remaining = deadline.saturating_duration_since(Instant::now());
    let timeout_ms = remaining.as_millis().max(1000) as i32;

    if link.request(&node, &page_path, &[], timeout_ms).is_err() {
        print_last_error("link.request failed");
        return 1;
    }
    eprintln!("request sent for {page_path}");

    while Instant::now() < deadline {
        match Event::poll(&node, 500, &mut page_buf) {
            Ok(ev) => match ev.kind() {
                EventKind::RequestResponse => {
                    let data = ev.app_data();
                    println!("\n=== Page Content ({} bytes) ===", data.len());
                    if !data.is_empty() {
                        let _ = io::stdout().write_all(data);
                        if data[data.len() - 1] != b'\n' {
                            println!();
                        }
                    }
                    if ev.app_data_truncated() {
                        eprintln!("warning: response truncated to {PAGE_BUF_CAP} bytes");
                    }
                    println!("=== End of Page ===");
                    return 0;
                }
                EventKind::RequestFailed => {
                    eprintln!("request failed: {}", ev.error_message());
                    return 1;
                }
                EventKind::LinkClosed => {
                    eprintln!("link closed before response");
                    return 1;
                }
                _ => {}
            },
            Err(Error::Timeout) => {}
            Err(_) => {
                print_last_error("Event::poll failed");
                return 1;
            }
        }
    }

    eprintln!("timed out waiting for page response");
    1
}

fn main() {
    let args: Vec<String> = env::args().collect();
    let argv0 = args.first().map(String::as_str).unwrap_or("rns-page-fetch");

    let mut config_path = String::new();
    let mut timeout_sec = DEFAULT_TIMEOUT_SEC;
    let mut target = String::new();

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "-c" if i + 1 < args.len() => {
                i += 1;
                config_path = args[i].clone();
            }
            "-t" if i + 1 < args.len() => {
                i += 1;
                timeout_sec = args[i].parse().unwrap_or(0);
                if timeout_sec == 0 {
                    eprintln!("timeout must be positive");
                    std::process::exit(1);
                }
            }
            "-h" | "--help" => {
                usage(argv0);
                return;
            }
            arg if arg.starts_with('-') => {
                eprintln!("unknown option: {arg}");
                usage(argv0);
                std::process::exit(1);
            }
            _ => {
                if !target.is_empty() {
                    eprintln!("extra argument: {}", args[i]);
                    usage(argv0);
                    std::process::exit(1);
                }
                target = args[i].clone();
            }
        }
        i += 1;
    }

    if target.is_empty() || config_path.is_empty() {
        usage(argv0);
        std::process::exit(1);
    }

    std::process::exit(run(&config_path, &target, timeout_sec));
}
