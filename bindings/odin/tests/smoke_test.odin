// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

import "core:os"
import "core:testing"

import rns "rns:rns"

@(test)
test_version :: proc(t: ^testing.T) {
	v := rns.version()
	testing.expect_value(t, v, rns.API_VERSION)
}

@(test)
test_node_lifecycle :: proc(t: ^testing.T) {
	node, err := rns.node_create("")
	if !expect_ok(t, err) {
		return
	}
	defer rns.node_destroy(node)

	if !expect_ok(t, rns.node_start(node)) {
		return
	}

	_, poll_err := rns.event_poll(node, 10)
	testing.expect_value(t, poll_err, rns.Error.Timeout)

	if !expect_ok(t, rns.node_stop(node)) {
		return
	}
}

@(test)
test_invalid_handle :: proc(t: ^testing.T) {
	err := rns.node_start(rns.Node(0))
	testing.expect(t, err == .Invalid_Handle || err == .Invalid_Arg || err == .Internal)
}

@(test)
test_added_abi_surface :: proc(t: ^testing.T) {
	node, err := rns.node_create("")
	if !expect_ok(t, err) {
		return
	}
	defer rns.node_destroy(node)
	testing.expect_value(t, rns.node_reload_config(node), rns.Error.Invalid_Arg)
	config_path := "/tmp/rns-odin-reload.conf"
	testing.expect(t, os.write_entire_file(config_path, "[reticulum]\nenable_transport = no\nin_memory_storage = yes\n") == nil)
	configured, configured_err := rns.node_create(config_path)
	if expect_ok(t, configured_err) {
		testing.expect_value(t, rns.node_reload_config(configured), rns.Error.Ok)
		rns.node_destroy(configured)
	}
	os.remove(config_path)
	unknown: [rns.HASH_LEN]u8
	_, link_err := rns.link_from_id(node, unknown[:])
	testing.expect_value(t, link_err, rns.Error.Not_Found)
}
