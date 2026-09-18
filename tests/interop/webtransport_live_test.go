// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Go-Go WebTransport interface interop. Go-only extension.
// Set RUN_LIVE_INTEROP=1 to enable.

//go:build !js

package interop

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

func wtServerPort(t *testing.T, srv *interfaces.WebTransportServerInterface) int {
	t.Helper()
	addr := srv.ListenAddr()
	if addr == nil {
		t.Fatal("nil listen addr")
	}
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		t.Fatalf("unexpected addr %T", addr)
	}
	return ua.Port
}

func waitWTOnline(t *testing.T, cli *interfaces.WebTransportClientInterface, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cli.IsOnline() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("webtransport client not online")
}

// TestLiveInteropWebTransportGoGoEcho sends a frame client->server and
// verifies the datagram lands on the server packet callback.
func TestLiveInteropWebTransportGoGoEcho(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	srv, err := interfaces.NewWebTransportServerInterface("live_wt_srv", "127.0.0.1", port, "/rns", interfaces.WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = wtServerPort(t, srv)

	cli, err := interfaces.NewWebTransportClientInterfaceWithRetries("live_wt_cli", "127.0.0.1", port, "/rns", true, -1, interfaces.WebTransportClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitWTOnline(t, cli, 15*time.Second)

	payload := []byte{0x01, 0x02, 0x7e, 0x7d, 0xaa}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatalf("send: %v", err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for server packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
}

// TestLiveInteropWebTransportBidirectional sends a payload each way over one
// WebTransport session.
func TestLiveInteropWebTransportBidirectional(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	srv, err := interfaces.NewWebTransportServerInterface("live_wt_srv2", "127.0.0.1", port, "/rns", interfaces.WebTransportServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = wtServerPort(t, srv)

	cli, err := interfaces.NewWebTransportClientInterfaceWithRetries("live_wt_cli2", "127.0.0.1", port, "/rns", true, -1, interfaces.WebTransportClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitWTOnline(t, cli, 15*time.Second)

	var srvGot atomic.Value
	srvDone := make(chan struct{})
	srv.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		srvGot.Store(append([]byte(nil), data...))
		close(srvDone)
	})
	var cliGot atomic.Value
	cliDone := make(chan struct{})
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		cliGot.Store(append([]byte(nil), data...))
		close(cliDone)
	})

	c2s := []byte("wt-c2s")
	if err := cli.Send(c2s, ""); err != nil {
		t.Fatalf("send c2s: %v", err)
	}
	select {
	case <-srvDone:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not receive client payload")
	}
	if recv, _ := srvGot.Load().([]byte); !bytes.Equal(recv, c2s) {
		t.Fatalf("server got %x want %x", recv, c2s)
	}

	s2c := []byte("wt-s2c")
	if err := srv.Send(s2c, ""); err != nil {
		t.Fatalf("send s2c: %v", err)
	}
	select {
	case <-cliDone:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not receive server payload")
	}
	if recv, _ := cliGot.Load().([]byte); !bytes.Equal(recv, s2c) {
		t.Fatalf("client got %x want %x", recv, s2c)
	}
}
