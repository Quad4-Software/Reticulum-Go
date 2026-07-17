// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//
// NomadNet-style pageserver over the Odin librns bindings.
// Usage: odin-pageserver -c config [-i identity] [-a announce_sec] [-p page_file] [-P request_path]

package main

import "core:fmt"
import "core:os"
import "core:strconv"
import "core:strings"
import "core:time"

import rns "rns:rns"

DEFAULT_ANNOUNCE_SEC :: 900
DEFAULT_PAGE_PATH :: "/page/index.mu"
DEFAULT_FILE_PATH :: "/file/test.txt"
DEFAULT_PAGE_FILE :: "pages/index.mu"
DEFAULT_FILE_FILE :: "files/test.txt"
DEFAULT_IDENTITY_PATH :: "identity"
REQ_DATA_CAP :: 64 * 1024

FALLBACK_PAGE ::
	"> Odin pageserver\n\n" +
	"librns via Reticulum-Go\n\n" +
	"Fallback page (file not found).\n\n" +
	"`[Home`:/page/index.mu]\n" +
	"`[Download Test File`:/file/test.txt]`_`f\n\n" +
	"---\n"

FALLBACK_FILE :: "Test file from Reticulum-Go node!\n"

usage :: proc(argv0: string) {
	fmt.eprintf(
		"Usage: %s -c config [-i identity] [-a announce_sec] [-p page_file] [-f file] [-P request_path]\n" +
		"\n" +
		"Serve a NomadNet-compatible /page/ handler over librns.\n" +
		"Destination: nomadnetwork.node\n" +
		"Announce app_data name: librns-odin-pageserver\n" +
		"\n" +
		"Options:\n" +
		"  -c path   Reticulum config file (required)\n" +
		"  -i path   Persistent identity file (default %s)\n" +
		"            Loaded when present, otherwise generated and saved\n" +
		"  -a sec    Announce interval seconds (default %d, 0 = once)\n" +
		"  -p file   Micron page file to serve (default %s)\n" +
		"  -f file   Download file to serve at /file/test.txt (default %s)\n" +
		"  -P path   Request path to register (default %s)\n",
		argv0,
		DEFAULT_IDENTITY_PATH,
		DEFAULT_ANNOUNCE_SEC,
		DEFAULT_PAGE_FILE,
		DEFAULT_FILE_FILE,
		DEFAULT_PAGE_PATH,
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

load_page :: proc(path: string) -> []u8 {
	data, err := os.read_entire_file(path, context.allocator)
	if err != nil {
		fmt.eprintf("warning: could not read %s, using built-in page\n", path)
		return transmute([]u8)strings.clone(FALLBACK_PAGE)
	}
	return data
}

load_file :: proc(path: string) -> []u8 {
	data, err := os.read_entire_file(path, context.allocator)
	if err != nil {
		fmt.eprintf("warning: could not read %s, using built-in file\n", path)
		return transmute([]u8)strings.clone(FALLBACK_FILE)
	}
	return data
}

load_or_create_identity :: proc(path: string) -> (identity: rns.Identity, err: rns.Error) {
	identity, err = rns.identity_load(path)
	if err == .Ok {
		fmt.eprintf("loaded identity from %s\n", path)
		return identity, .Ok
	}
	identity, err = rns.identity_generate()
	if err != .Ok {
		return 0, err
	}
	if save_err := rns.identity_save(identity, path); save_err != .Ok {
		_ = rns.identity_destroy(identity)
		return 0, save_err
	}
	fmt.eprintf("created and saved identity to %s\n", path)
	return identity, .Ok
}

run :: proc(config_path, identity_path, page_file, file_file, request_path, file_path: string, announce_sec: int) -> int {
	ver := rns.version()
	if ver != rns.API_VERSION {
		fmt.eprintf("librns version mismatch: got %s want %s\n", ver, rns.API_VERSION)
		return 1
	}

	page_body := load_page(page_file)
	defer delete(page_body)
	file_body := load_file(file_file)
	defer delete(file_body)

	node, nerr := rns.node_create(config_path)
	if nerr != .Ok {
		print_last_error("rns.node_create failed")
		return 1
	}
	defer rns.node_destroy(node)

	identity, ierr := load_or_create_identity(identity_path)
	if ierr != .Ok {
		print_last_error("identity load/create failed")
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

	dest, derr := rns.destination_create(node, 0, "nomadnetwork", {"node"}, true)
	if derr != .Ok {
		print_last_error("rns.destination_create failed")
		return 1
	}
	defer rns.destination_destroy(dest)

	if rns.destination_register_request_handler(dest, request_path) != .Ok {
		print_last_error("rns.destination_register_request_handler failed")
		return 1
	}
	if rns.destination_register_request_handler(dest, file_path) != .Ok {
		print_last_error("rns.destination_register_request_handler file failed")
		return 1
	}

	dest_hash, herr := rns.destination_hash(dest)
	if herr != .Ok {
		print_last_error("rns.destination_hash failed")
		return 1
	}
	dest_hex, hex_ok := rns.hash_to_hex(dest_hash[:])
	if !hex_ok {
		fmt.eprintln("failed to encode destination hash")
		return 1
	}
	defer delete(dest_hex)

	fmt.printf("DEST_HASH=%s\n", dest_hex)
	fmt.printf("REQUEST_PATH=%s\n", request_path)
	fmt.printf("FILE_PATH=%s\n", file_path)
	fmt.eprintf("librns %s pageserver listening as nomadnetwork.node\n", ver)
	fmt.eprintf("announce name=librns-odin-pageserver interval=%ds\n", announce_sec)
	fmt.eprintf("serving %d bytes from %s\n", len(page_body), page_file)
	fmt.eprintf("serving %d bytes from %s as %s\n", len(file_body), file_file, file_path)

	app_data := transmute([]u8)string("librns-odin-pageserver")
	if rns.destination_announce(dest, app_data) != .Ok {
		print_last_error("rns.destination_announce failed")
	} else {
		fmt.eprintln("announce sent")
	}

	req_buf := make([]u8, REQ_DATA_CAP)
	defer delete(req_buf)

	announce_every := time.Duration(announce_sec) * time.Second
	last_announce := time.tick_now()

	for {
		if announce_sec > 0 && time.tick_since(last_announce) >= announce_every {
			if rns.destination_announce(dest, app_data) == .Ok {
				fmt.eprintln("announce refreshed")
			}
			last_announce = time.tick_now()
		}

		ev, err := rns.event_poll(node, 200, req_buf)
		if err == .Timeout {
			continue
		}
		if err != .Ok {
			print_last_error("rns.event_poll failed")
			return 1
		}

		switch ev.kind {
		case .Link_Established:
			fmt.eprintln("inbound link established")
		case .Link_Closed:
			fmt.eprintln("link closed")
		case .Request_Incoming:
			path := rns.event_path(&ev)
			fmt.eprintf("request incoming path=%s\n", path)
			req_id := rns.event_request_id(&ev)
			if path == request_path {
				if rns.request_respond(node, req_id, page_body) != .Ok {
					print_last_error("rns.request_respond failed")
				} else {
					fmt.eprintf("served %s (%d bytes)\n", request_path, len(page_body))
				}
			} else if path == file_path {
				if rns.request_respond_file(node, req_id, "test.txt", file_body) != .Ok {
					print_last_error("rns.request_respond_file failed")
				} else {
					fmt.eprintf("served %s (%d bytes)\n", file_path, len(file_body))
				}
			} else {
				msg := transmute([]u8)string("page not found\n")
				if rns.request_respond(node, req_id, msg) != .Ok {
					print_last_error("rns.request_respond failed")
				}
			}
		case .None, .Announce, .Link_Failed, .Link_Data, .Request_Response, .Request_Failed:
		}
	}
}

main :: proc() {
	config_path: string
	identity_path := DEFAULT_IDENTITY_PATH
	page_file := DEFAULT_PAGE_FILE
	file_file := DEFAULT_FILE_FILE
	request_path := DEFAULT_PAGE_PATH
	file_path := DEFAULT_FILE_PATH
	announce_sec := DEFAULT_ANNOUNCE_SEC

	args := os.args
	for i := 1; i < len(args); i += 1 {
		arg := args[i]
		if arg == "-c" && i + 1 < len(args) {
			i += 1
			config_path = args[i]
			continue
		}
		if arg == "-i" && i + 1 < len(args) {
			i += 1
			identity_path = args[i]
			continue
		}
		if arg == "-a" && i + 1 < len(args) {
			i += 1
			n, ok := strconv.parse_int(args[i])
			if !ok || n < 0 {
				fmt.eprintln("announce interval must be >= 0")
				os.exit(1)
			}
			announce_sec = n
			continue
		}
		if arg == "-p" && i + 1 < len(args) {
			i += 1
			page_file = args[i]
			continue
		}
		if arg == "-f" && i + 1 < len(args) {
			i += 1
			file_file = args[i]
			continue
		}
		if arg == "-P" && i + 1 < len(args) {
			i += 1
			request_path = args[i]
			continue
		}
		if arg == "-h" || arg == "--help" {
			usage(args[0])
			os.exit(0)
		}
		fmt.eprintf("unknown option: %s\n", arg)
		usage(args[0])
		os.exit(1)
	}

	if config_path == "" {
		usage(args[0])
		os.exit(1)
	}

	os.exit(run(config_path, identity_path, page_file, file_file, request_path, file_path, announce_sec))
}
