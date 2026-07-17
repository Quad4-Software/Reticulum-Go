// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sharedinstance

import (
	"net"
	"strconv"
	"testing"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
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
		t.Fatalf("link_count = %d; want 0", count)
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
