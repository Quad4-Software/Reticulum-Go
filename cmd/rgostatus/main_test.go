// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"quad4/reticulum-go/pkg/cli"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/sharedinstance"
	"quad4/reticulum-go/pkg/transport"
)

func TestRgostatusAgainstRPC(t *testing.T) {
	cfgDir := t.TempDir()
	cfg := &common.ReticulumConfig{EnableTransport: false}
	tr := transport.NewTransport(cfg)
	defer tr.Close()

	port := freePort(t)
	cfg.InstanceControlPort = port
	srv, err := sharedinstance.StartRPCServer(cfg, tr)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	auth := tr.RPCAuthKey()
	configBody := "[reticulum]\n" +
		"enable_transport = no\n" +
		"share_instance = yes\n" +
		"instance_control_port = " + strconv.Itoa(port) + "\n" +
		"rpc_key = " + hexKey(auth) + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config"), []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	oldOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := cli.RunStatus([]string{"-config", cfgDir, "-json"})
	_ = w.Close()
	os.Stdout = oldOut
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if code != 0 {
		t.Fatalf("exit=%d out=%s", code, buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("transport_id")) {
		t.Fatalf("output=%s", buf.String())
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func hexKey(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}
