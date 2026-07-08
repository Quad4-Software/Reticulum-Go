// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package i2p

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func liveI2POrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LIVE_I2P") != "1" {
		t.Skip("set RUN_LIVE_I2P=1 to run live I2P/SAM tests against a local router")
	}
	addr := SAMAddressFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Skipf("SAM not reachable at %s: %v", addr, err)
	}
	_ = conn.Close()
}

func TestLiveSAMHello(t *testing.T) {
	liveI2POrSkip(t)
	c := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := c.dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
}

func TestLiveSAMDestGenerate(t *testing.T) {
	liveI2POrSkip(t)
	c := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dest, err := c.DestGenerate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dest.Base32() == "" || len(dest.Base32()) != 52 {
		t.Fatalf("unexpected b32 %q", dest.Base32())
	}
	if dest.PrivateKeyB64() == "" {
		t.Fatal("missing private key")
	}
}

func TestLiveSAMStreamLoopback(t *testing.T) {
	liveI2POrSkip(t)
	c := NewClient("")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	dest, err := c.DestGenerate(ctx)
	if err != nil {
		t.Fatalf("dest generate: %v", err)
	}
	serverID := GenerateSessionID()
	serverSess, err := c.OpenSession(ctx, serverID, dest.PrivateKeyB64())
	if err != nil {
		t.Fatalf("server session: %v", err)
	}
	defer serverSess.Close()

	acceptDone := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := c.StreamAccept(ctx, serverID)
		if err != nil {
			acceptErr <- err
			return
		}
		acceptDone <- conn
	}()

	clientID := GenerateSessionID()
	clientSess, err := c.OpenSession(ctx, clientID, transientDest)
	if err != nil {
		t.Fatalf("client session: %v", err)
	}
	defer clientSess.Close()
	clientConn, err := c.StreamConnect(ctx, clientID, dest.Base64())
	if err != nil {
		t.Fatalf("stream connect: %v", err)
	}
	defer clientConn.Close()

	var serverConn net.Conn
	select {
	case serverConn = <-acceptDone:
	case err := <-acceptErr:
		t.Fatalf("stream accept: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for stream accept")
	}
	defer serverConn.Close()

	payload := []byte("reticulum-go-i2p-live-test")
	if _, err := clientConn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	br := bufio.NewReader(serverConn)
	if _, err := br.ReadString('\n'); err != nil {
		t.Fatalf("read peer destination line: %v", err)
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != string(payload) {
		t.Fatalf("payload mismatch: %q", buf)
	}
}

func TestLiveServerClientTunnel(t *testing.T) {
	liveI2POrSkip(t)
	dir := t.TempDir()
	ctrl := NewController(filepath.Join(dir, "i2p"), "")
	defer ctrl.Stop()

	localPort, err := ctrl.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	transportID := []byte("live-test-transport-id")
	tun, dest, err := ctrl.StartServerTunnel("live-test", transportID, localPort)
	if err != nil {
		t.Fatalf("server tunnel: %v", err)
	}
	if dest == nil || dest.Base32() == "" {
		t.Fatal("missing server destination")
	}

	ln, err := net.Listen("tcp", tun.LocalAddr())
	if err != nil {
		t.Fatalf("listen on tunnel local addr %s: %v", tun.LocalAddr(), err)
	}
	defer ln.Close()

	echoDone := make(chan struct{})
	go func() {
		defer close(echoDone)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		n, _ := conn.Read(buf)
		_, _ = conn.Write(buf[:n])
	}()

	clientPort, err := ctrl.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	clientTun, err := ctrl.StartClientTunnel(dest.Base32()+".b32.i2p", clientPort)
	if err != nil {
		t.Fatalf("client tunnel: %v", err)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var conn net.Conn
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		d, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", clientTun.LocalAddr())
		if err == nil {
			conn = d
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("timed out dialing client tunnel")
	}
	defer conn.Close()

	msg := []byte("tunnel-echo")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, out); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(out) != string(msg) {
		t.Fatalf("echo mismatch: %q", out)
	}
	<-echoDone
}
