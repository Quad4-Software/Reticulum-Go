// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import "core:fmt"
import "core:os"

import rns "rns:rns"

main :: proc() {
	ver := rns.version()
	if ver != rns.API_VERSION {
		fmt.eprintf("unexpected version: %s\n", ver)
		os.exit(1)
	}

	node, err := rns.node_create("")
	if err != .Ok {
		fmt.eprintf("node_create failed: %v\n", err)
		os.exit(1)
	}
	defer rns.node_destroy(node)

	if err = rns.node_start(node); err != .Ok {
		fmt.eprintf("node_start failed: %v\n", err)
		os.exit(1)
	}
	defer rns.node_stop(node)

	_, perr := rns.event_poll(node, 10)
	if perr != .Timeout {
		fmt.eprintf("expected timeout poll on idle node\n")
		os.exit(1)
	}

	fmt.println("odin-smoke ok")
}
