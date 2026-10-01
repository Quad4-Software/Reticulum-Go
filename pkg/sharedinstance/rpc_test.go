// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"net"
	"strconv"
	"testing"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/profiler"
	"github.com/Quad4-Software/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func TestRPCServerLinkCountAfterAuth(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: false}
	tr := transport.NewTransport(cfg)
	defer tr.Close()

	port := freeTCPPort(t)
	cfg.InstanceControlPort = port
	cfg.SharedInstanceType = common.SharedInstanceTCP

	srv, err := StartRPCServer(cfg, tr)
	if err != nil {
		t.Fatalf("StartRPCServer: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	authkey := tr.RPCAuthKey()
	if len(authkey) == 0 {
		t.Fatal("empty rpc auth key")
	}
	if err := AuthenticateClient(conn, authkey); err != nil {
		t.Fatalf("AuthenticateClient: %v", err)
	}

	call, err := msgpack.Marshal(map[string]any{"get": "link_count"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := sendBytes(conn, call); err != nil {
		t.Fatalf("send: %v", err)
	}
	resp, err := recvBytes(conn, 1<<20)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	var count int
	if err := msgpack.Unmarshal(resp, &count); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if count != 0 {
		t.Fatalf("link_count = %d. Want 0", count)
	}
}

func TestRPCHandlerProfilingResults(t *testing.T) {
	profiler.Reset()
	defer profiler.Reset()

	cfg := &common.ReticulumConfig{EnableTransport: false, InMemoryStorage: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	h := &RPCHandler{Transport: tr}

	if got := h.Handle(map[string]any{"get": "profiling_results"}); got != nil {
		t.Fatalf("expected nil before captures, got %#v", got)
	}

	profiler.Do("rpc.probe", func() {})
	got := h.Handle(map[string]any{"get": "profiling_results"})
	m, ok := got.(map[string]profiler.TagResult)
	if !ok || m["rpc.probe"].StatsAll == nil {
		t.Fatalf("unexpected profiling payload: %#v", got)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestRPCHandlerManage covers the RNS 1.5.5 interface management RPC path
// (rnstatus --attach/--detach/--reload). Without a Manage hook the handler
// answers false; with one it forwards the action and interface name and
// preserves the Python tri-state (true/false/nil).
func TestRPCHandlerManage(t *testing.T) {
	cfg := &common.ReticulumConfig{EnableTransport: false, InMemoryStorage: true}
	tr := transport.NewTransport(cfg)
	defer tr.Close()
	h := &RPCHandler{Transport: tr}

	for _, action := range []string{"attach_interface", "detach_interface", "reload_interface"} {
		got := h.Handle(map[string]any{"manage": action, "name": "udp_test"})
		if b, ok := got.(bool); !ok || b {
			t.Fatalf("no-hook manage %s: %#v want false", action, got)
		}
	}

	var gotAction, gotName string
	returns := map[string]any{
		"attach_interface": true,
		"detach_interface": false,
		"reload_interface": nil,
	}
	h.Manage = func(action, name string) any {
		gotAction, gotName = action, name
		return returns[action]
	}
	for action, want := range returns {
		got := h.Handle(map[string]any{"manage": action, "name": "udp_test"})
		if gotAction != action || gotName != "udp_test" {
			t.Fatalf("manage forwarded %s/%s want %s/udp_test", gotAction, gotName, action)
		}
		switch want {
		case nil:
			if got != nil {
				t.Fatalf("manage %s: %#v want nil", action, got)
			}
		default:
			if got != want {
				t.Fatalf("manage %s: %#v want %#v", action, got, want)
			}
		}
	}
	// Unknown manage actions are ignored.
	if got := h.Handle(map[string]any{"manage": "bogus", "name": "x"}); got != nil {
		t.Fatalf("bogus manage: %#v want nil", got)
	}
}
