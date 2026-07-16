// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

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
