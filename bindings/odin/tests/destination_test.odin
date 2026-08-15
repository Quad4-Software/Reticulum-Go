// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

import "core:path/filepath"
import "core:testing"

import rns "rns:rns"

@(test)
test_destination_create_announce_hash :: proc(t: ^testing.T) {
	port, ok := free_udp_port()
	if !testing.expect(t, ok) {
		return
	}
	dir := make_temp_dir(t)
	if !testing.expect(t, write_udp_peer_config(dir, port, port)) {
		return
	}
	cfg, _ := filepath.join({dir, "config"})
	defer delete(cfg)

	node, nerr := rns.node_create(cfg)
	if !expect_ok(t, nerr) {
		return
	}
	defer rns.node_destroy(node)

	id, ierr := rns.identity_generate()
	if !expect_ok(t, ierr) {
		return
	}
	defer rns.identity_destroy(id)

	if !expect_ok(t, rns.node_set_identity(node, id)) {
		return
	}
	if !expect_ok(t, rns.node_start(node)) {
		return
	}
	defer rns.node_stop(node)

	dest, derr := rns.destination_create(node, 0, "odin-rns", {"chat"}, true)
	if !expect_ok(t, derr) {
		return
	}
	defer rns.destination_destroy(dest)

	if !expect_ok(t, rns.destination_announce(dest, transmute([]u8)string("hello"))) {
		return
	}

	hash, herr := rns.destination_hash(dest)
	if !expect_ok(t, herr) {
		return
	}
	testing.expect_value(t, len(hash), rns.HASH_LEN)

	entries: [8]rns.Path_Entry
	_, perr := rns.path_table(node, entries[:], -1)
	testing.expect(t, perr == .Ok || perr == .Not_Found)
}
