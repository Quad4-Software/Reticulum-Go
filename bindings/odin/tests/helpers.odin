// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rns_tests

import "core:fmt"
import "core:net"
import "core:os"
import "core:path/filepath"
import "core:testing"
import "core:time"

import rns "rns:rns"

expect_ok :: proc(t: ^testing.T, err: rns.Error, loc := #caller_location) -> bool {
	if err == .Ok {
		return true
	}
	msg, _ := rns.last_error()
	defer delete(msg)
	return testing.expectf(
		t,
		false,
		"rns error %v (%s): %s",
		err,
		rns.error_string(err),
		msg,
		loc = loc,
	)
}

free_udp_port :: proc() -> (port: int, ok: bool) {
	sock, err := net.make_unbound_udp_socket(.IP4)
	if err != nil {
		return 0, false
	}
	defer net.close(sock)
	if net.bind(sock, net.Endpoint{address = net.IP4_Loopback, port = 0}) != nil {
		return 0, false
	}
	ep, info_err := net.bound_endpoint(sock)
	if info_err != nil {
		return 0, false
	}
	return ep.port, true
}

write_udp_peer_config :: proc(dir: string, listen: int, peer: int) -> bool {
	cfg := fmt.tprintf(
		"[reticulum]\n" +
		"enable_transport = yes\n" +
		"share_instance = no\n" +
		"\n" +
		"[interfaces]\n" +
		"  [[UDP]]\n" +
		"    type = UDPInterface\n" +
		"    enabled = yes\n" +
		"    listen_ip = 127.0.0.1\n" +
		"    listen_port = %d\n" +
		"    target_host = 127.0.0.1\n" +
		"    target_port = %d\n",
		listen,
		peer,
	)
	path, err := filepath.join({dir, "config"})
	if err != nil {
		return false
	}
	defer delete(path)
	return os.write_entire_file(path, transmute([]byte)cfg) == nil
}

poll_until :: proc(
	t: ^testing.T,
	node: rns.Node,
	want: rns.Event_Kind,
	timeout: time.Duration,
	app_buf: []u8 = nil,
) -> (event: rns.Event, ok: bool) {
	deadline := time.tick_add(time.tick_now(), timeout)
	for time.tick_diff(time.tick_now(), deadline) > 0 {
		ev, err := rns.event_poll(node, 50, app_buf)
		if err == .Timeout {
			continue
		}
		if !expect_ok(t, err) {
			return {}, false
		}
		if ev.kind == want {
			return ev, true
		}
	}
	testing.expectf(t, false, "timed out waiting for event %v", want)
	return {}, false
}

make_temp_dir :: proc(t: ^testing.T) -> string {
	dir, err := os.make_directory_temp("", "rns-odin-*", context.allocator)
	if err != nil {
		testing.fail_now(t, "make_directory_temp failed")
	}
	heap := new(string)
	heap^ = dir
	testing.cleanup(t, proc(data: rawptr) {
		p := (^string)(data)
		os.remove_all(p^)
		delete(p^)
		free(p)
	}, heap)
	return dir
}
