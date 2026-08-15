// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

import "core:bytes"
import "core:path/filepath"
import "core:testing"
import "core:time"

import rns "rns:rns"

@(test)
test_link_announce_open_send_over_udp :: proc(t: ^testing.T) {
	port_a, ok_a := free_udp_port()
	port_b, ok_b := free_udp_port()
	if !testing.expect(t, ok_a && ok_b) {
		return
	}

	dir_a := make_temp_dir(t)
	dir_b := make_temp_dir(t)
	if !testing.expect(t, write_udp_peer_config(dir_a, port_a, port_b)) {
		return
	}
	if !testing.expect(t, write_udp_peer_config(dir_b, port_b, port_a)) {
		return
	}

	cfg_a, _ := filepath.join({dir_a, "config"})
	cfg_b, _ := filepath.join({dir_b, "config"})
	defer delete(cfg_a)
	defer delete(cfg_b)

	node_a, err_a := rns.node_create(cfg_a)
	if !expect_ok(t, err_a) {
		return
	}
	defer rns.node_destroy(node_a)

	node_b, err_b := rns.node_create(cfg_b)
	if !expect_ok(t, err_b) {
		return
	}
	defer rns.node_destroy(node_b)

	id_a, ierr_a := rns.identity_generate()
	if !expect_ok(t, ierr_a) {
		return
	}
	defer rns.identity_destroy(id_a)

	id_b, ierr_b := rns.identity_generate()
	if !expect_ok(t, ierr_b) {
		return
	}
	defer rns.identity_destroy(id_b)

	if !expect_ok(t, rns.node_set_identity(node_a, id_a)) {
		return
	}
	if !expect_ok(t, rns.node_set_identity(node_b, id_b)) {
		return
	}
	if !expect_ok(t, rns.node_start(node_a)) {
		return
	}
	defer rns.node_stop(node_a)
	if !expect_ok(t, rns.node_start(node_b)) {
		return
	}
	defer rns.node_stop(node_b)

	dest_a, derr := rns.destination_create(node_a, 0, "odin-rns", {"link"}, true)
	if !expect_ok(t, derr) {
		return
	}
	defer rns.destination_destroy(dest_a)

	app_payload := transmute([]u8)string("odin-app")
	if !expect_ok(t, rns.destination_announce(dest_a, app_payload)) {
		return
	}

	app_buf: [256]u8
	announce, got := poll_until(t, node_b, .Announce, 5 * time.Second, app_buf[:])
	if !got {
		return
	}
	testing.expect(t, bytes.equal(rns.event_app_data(&announce), app_payload))

	dest_hash, herr := rns.destination_hash(dest_a)
	if !expect_ok(t, herr) {
		return
	}
	testing.expect(t, bytes.equal(rns.event_destination_hash(&announce), dest_hash[:]))

	link_b, lerr := rns.link_open(node_b, dest_hash[:])
	if !expect_ok(t, lerr) {
		return
	}
	defer rns.link_close(link_b)

	_, ok_est_b := poll_until(t, node_b, .Link_Established, 5 * time.Second)
	if !ok_est_b {
		return
	}
	_, ok_est_a := poll_until(t, node_a, .Link_Established, 5 * time.Second)
	if !ok_est_a {
		return
	}

	payload := transmute([]u8)string("odin-link-payload")
	if !expect_ok(t, rns.link_send(link_b, payload)) {
		return
	}

	data_buf: [256]u8
	data_ev, ok_data := poll_until(t, node_a, .Link_Data, 5 * time.Second, data_buf[:])
	if !ok_data {
		return
	}
	testing.expect(t, bytes.equal(rns.event_app_data(&data_ev), payload))
}
