// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style page fetch over the Odin librns bindings.
// Usage: odin-page-fetch [-c config] [-t timeout_sec] <dest_hash>:<page_path>

package main

import "core:fmt"
import "core:mem"
import "core:os"
import "core:strconv"
import "core:strings"
import "core:time"

import rns "rns:rns"

PAGE_BUF_CAP :: 512 * 1024
PATH_TABLE_CAP :: 256
DEFAULT_TIMEOUT_SEC :: 60
PATH_RETRY :: 2 * time.Second

usage :: proc(argv0: string) {
	fmt.eprintf(
		"Usage: %s [-c config] [-t timeout_sec] <dest_hash>:<page_path>\n" +
		"\n" +
		"Fetch a NomadNet / pageserver page over librns (Odin bindings).\n" +
		"\n" +
		"Options:\n" +
		"  -c path   Reticulum config file (required for network interfaces)\n" +
		"  -t sec    Overall timeout in seconds (default %d)\n" +
		"\n" +
		"Example:\n" +
		"  %s -c config.example 92798ea245a0afcfa559348e42d628c6:/page/index.mu\n",
		argv0,
		DEFAULT_TIMEOUT_SEC,
		argv0,
	)
}

print_last_error :: proc(what: string) {
	msg, err := rns.last_error()
	defer delete(msg)
	if err == .Ok && msg != "" {
		fmt.eprintf("%s: %s\n", what, msg)
	} else {
		fmt.eprintf("%s\n", what)
	}
}

hash_eq :: proc(a, b: []u8) -> bool {
	return len(a) == rns.HASH_LEN && len(b) == rns.HASH_LEN && mem.compare(a, b) == 0
}

path_known :: proc(node: rns.Node, dest: []u8) -> bool {
	table: [PATH_TABLE_CAP]rns.Path_Entry
	n, err := rns.path_table(node, table[:])
	if err != .Ok {
		return false
	}
	for i in 0 ..< n {
		entry := table[i]
		if hash_eq(entry.hash[:entry.hash_len], dest) {
			return true
		}
	}
	return false
}

parse_target :: proc(target: string) -> (hash: [rns.HASH_LEN]u8, page_path: string, ok: bool) {
	colon := strings.index_byte(target, ':')
	if colon <= 0 || colon + 1 >= len(target) {
		return {}, "", false
	}
	hash_hex := target[:colon]
	page_path = target[colon + 1:]
	decoded, decode_ok := rns.hex_to_hash(hash_hex)
	if !decode_ok || len(decoded) != rns.HASH_LEN {
		delete(decoded)
		return {}, "", false
	}
	copy(hash[:], decoded)
	delete(decoded)
	return hash, page_path, true
}

run :: proc(config_path: string, target: string, timeout_sec: int) -> int {
	ver := rns.version()
	if ver != rns.API_VERSION {
		fmt.eprintf("librns version mismatch: got %s want %s\n", ver, rns.API_VERSION)
		return 1
	}

	dest_hash, page_path, ok := parse_target(target)
	if !ok {
		fmt.eprintln("target must be <32-hex-dest>:<page_path>")
		return 1
	}
	dest_hex, hex_ok := rns.hash_to_hex(dest_hash[:])
	if !hex_ok {
		fmt.eprintln("failed to encode destination hash")
		return 1
	}
	defer delete(dest_hex)

	node, nerr := rns.node_create(config_path)
	if nerr != .Ok {
		print_last_error("rns.node_create failed")
		return 1
	}
	defer rns.node_destroy(node)

	identity, ierr := rns.identity_generate()
	if ierr != .Ok {
		print_last_error("rns.identity_generate failed")
		return 1
	}
	defer rns.identity_destroy(identity)

	if rns.node_set_identity(node, identity) != .Ok {
		print_last_error("rns.node_set_identity failed")
		return 1
	}
	if rns.node_start(node) != .Ok {
		print_last_error("rns.node_start failed")
		return 1
	}
	defer rns.node_stop(node)

	fmt.printf("librns %s fetching %s from %s\n", ver, page_path, dest_hex)

	page_buf := make([]u8, PAGE_BUF_CAP)
	defer delete(page_buf)

	deadline := time.tick_add(time.tick_now(), time.Duration(timeout_sec) * time.Second)
	last_path_req := time.tick_now()
	need_path_req := true
	saw_announce := false
	link: rns.Link

	for time.tick_diff(time.tick_now(), deadline) > 0 && link == 0 {
		if need_path_req || time.tick_since(last_path_req) >= PATH_RETRY {
			if rns.path_request(node, dest_hash[:]) != .Ok {
				print_last_error("rns.path_request failed")
			}
			last_path_req = time.tick_now()
			need_path_req = false
			if path_known(node, dest_hash[:]) {
				fmt.eprintln("path known, waiting for destination identity announce")
			} else {
				fmt.eprintf("requesting path to %s\n", dest_hex)
			}
		}

		ev, err := rns.event_poll(node, 200, page_buf)
		if err == .Timeout {
			if saw_announce || path_known(node, dest_hash[:]) {
				opened, oerr := rns.link_open(node, dest_hash[:])
				if oerr == .Ok {
					link = opened
				}
			}
			continue
		}
		if err != .Ok {
			print_last_error("rns.event_poll failed")
			return 1
		}

		if ev.kind == .Announce && hash_eq(rns.event_destination_hash(&ev), dest_hash[:]) {
			saw_announce = true
			fmt.eprintf("announce for target (hops=%d)\n", ev.hops)
			opened, oerr := rns.link_open(node, dest_hash[:])
			if oerr != .Ok {
				print_last_error("rns.link_open after announce")
			} else {
				link = opened
			}
		} else if ev.kind == .Link_Failed {
			fmt.eprintf("link failed while opening: %s\n", rns.event_error_message(&ev))
		}
	}

	if link == 0 {
		fmt.eprintln("timed out before link open")
		return 1
	}
	defer rns.link_close(link)

	established := false
	for time.tick_diff(time.tick_now(), deadline) > 0 && !established {
		ev, err := rns.event_poll(node, 500, page_buf)
		if err == .Timeout {
			continue
		}
		if err != .Ok {
			print_last_error("rns.event_poll failed")
			return 1
		}
		switch ev.kind {
		case .Link_Established:
			established = true
			fmt.eprintln("link established")
		case .Link_Failed:
			fmt.eprintf("link establishment failed: %s\n", rns.event_error_message(&ev))
			return 1
		case .Link_Closed:
			fmt.eprintln("link closed before establish")
			return 1
		case .None, .Announce, .Link_Data, .Request_Incoming, .Request_Response, .Request_Failed:
		}
	}

	if !established {
		fmt.eprintln("timed out waiting for link establishment")
		return 1
	}

	remaining := time.tick_diff(time.tick_now(), deadline)
	timeout_ms := i32(remaining / time.Millisecond)
	if timeout_ms < 1000 {
		timeout_ms = 1000
	}

	_, req_err := rns.link_request(node, link, page_path, nil, timeout_ms)
	if req_err != .Ok {
		print_last_error("rns.link_request failed")
		return 1
	}
	fmt.eprintf("request sent for %s\n", page_path)

	for time.tick_diff(time.tick_now(), deadline) > 0 {
		ev, err := rns.event_poll(node, 500, page_buf)
		if err == .Timeout {
			continue
		}
		if err != .Ok {
			print_last_error("rns.event_poll failed")
			return 1
		}

		switch ev.kind {
		case .Request_Response:
			data := rns.event_app_data(&ev)
			fmt.printf("\n=== Page Content (%d bytes) ===\n", len(data))
			if len(data) > 0 {
				_, _ = os.write(os.stdout, data)
				if data[len(data) - 1] != '\n' {
					fmt.println()
				}
			}
			if ev.app_data_truncated != 0 {
				fmt.eprintf("warning: response truncated to %d bytes\n", PAGE_BUF_CAP)
			}
			fmt.println("=== End of Page ===")
			return 0
		case .Request_Failed:
			fmt.eprintf("request failed: %s\n", rns.event_error_message(&ev))
			return 1
		case .Link_Closed:
			fmt.eprintln("link closed before response")
			return 1
		case .None, .Announce, .Link_Established, .Link_Failed, .Link_Data, .Request_Incoming:
		}
	}

	fmt.eprintln("timed out waiting for page response")
	return 1
}

main :: proc() {
	config_path: string
	timeout_sec := DEFAULT_TIMEOUT_SEC
	target: string

	args := os.args
	for i := 1; i < len(args); i += 1 {
		arg := args[i]
		if arg == "-c" && i + 1 < len(args) {
			i += 1
			config_path = args[i]
			continue
		}
		if arg == "-t" && i + 1 < len(args) {
			i += 1
			n, n_ok := strconv.parse_int(args[i])
			if !n_ok || n <= 0 {
				fmt.eprintln("timeout must be positive")
				os.exit(1)
			}
			timeout_sec = n
			continue
		}
		if arg == "-h" || arg == "--help" {
			usage(args[0])
			os.exit(0)
		}
		if strings.has_prefix(arg, "-") {
			fmt.eprintf("unknown option: %s\n", arg)
			usage(args[0])
			os.exit(1)
		}
		if target != "" {
			fmt.eprintf("extra argument: %s\n", arg)
			usage(args[0])
			os.exit(1)
		}
		target = arg
	}

	if target == "" || config_path == "" {
		usage(args[0])
		os.exit(1)
	}

	os.exit(run(config_path, target, timeout_sec))
}
