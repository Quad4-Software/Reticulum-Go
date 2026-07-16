// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

import "core:testing"

import rns "rns:rns"

@(test)
test_identity_generate_hash_destroy :: proc(t: ^testing.T) {
	id, err := rns.identity_generate()
	if !expect_ok(t, err) {
		return
	}
	defer rns.identity_destroy(id)

	hex, herr := rns.identity_hash(id)
	if !expect_ok(t, herr) {
		return
	}
	defer delete(hex)
	testing.expect_value(t, len(hex), 32)

	node, nerr := rns.node_create("")
	if !expect_ok(t, nerr) {
		return
	}
	defer rns.node_destroy(node)

	if !expect_ok(t, rns.node_set_identity(node, id)) {
		return
	}
}

@(test)
test_identity_load_empty_path :: proc(t: ^testing.T) {
	_, err := rns.identity_load("")
	testing.expect_value(t, err, rns.Error.Invalid_Arg)
}
