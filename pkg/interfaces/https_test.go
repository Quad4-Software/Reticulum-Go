// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"bytes"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func freeHTTPSPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitHTTPSPeers(t *testing.T, srv *HTTPSServerInterface, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.PeerCount() >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d peers, have %d", n, srv.PeerCount())
}

func listenHTTPSPort(t *testing.T, srv *HTTPSServerInterface) int {
	t.Helper()
	addr := srv.ListenAddr()
	if addr == nil {
		t.Fatal("nil listen addr")
	}
	ta, ok := addr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected addr %T", addr)
	}
	return ta.Port
}

func TestNormalizeHTTPSPath(t *testing.T) {
	if got := normalizeHTTPSPath(""); got != "/rns" {
		t.Fatalf("empty: %q", got)
	}
	if got := normalizeHTTPSPath("/"); got != "/rns" {
		t.Fatalf("slash: %q", got)
	}
	if got := normalizeHTTPSPath("rns"); got != "/rns" {
		t.Fatalf("bare: %q", got)
	}
	if got := normalizeHTTPSPath("/rns/"); got != "/rns" {
		t.Fatalf("trim: %q", got)
	}
}

func TestHTTPSClientServerEcho(t *testing.T) {
	port := freeHTTPSPort(t)
	srv, err := NewHTTPSServerInterface("https_srv", "127.0.0.1", port, HTTPSServerOptions{
		Path:     "/rns",
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := srv.LeafSPKIPinHex()
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
	port = listenHTTPSPort(t, srv)

	cli, err := NewHTTPSClientInterfaceWithRetries("https_cli", "127.0.0.1", port, true, 20, HTTPSClientOptions{
		PeerKey:  pin,
		Path:     "/rns",
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitHTTPSPeers(t, srv, 1, 5*time.Second)

	payload := []byte{0x01, 0x02, 0x03, 0xaa, 0xbb}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for server packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
}

func TestHTTPSServerToClient(t *testing.T) {
	port := freeHTTPSPort(t)
	srv, err := NewHTTPSServerInterface("https_srv2", "127.0.0.1", port, HTTPSServerOptions{
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenHTTPSPort(t, srv)

	cli, err := NewHTTPSClientInterfaceWithRetries("https_cli2", "127.0.0.1", port, true, 20, HTTPSClientOptions{
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitHTTPSPeers(t, srv, 1, 5*time.Second)

	var got atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		wg.Done()
	})

	payload := []byte{0x10, 0x20, 0x30}
	if err := srv.Send(payload, ""); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for client packet")
	}
	recv, _ := got.Load().([]byte)
	if !bytes.Equal(recv, payload) {
		t.Fatalf("got %x want %x", recv, payload)
	}
}

func TestHTTPSPeerKeyPin(t *testing.T) {
	port := freeHTTPSPort(t)
	srv, err := NewHTTPSServerInterface("https_pin_srv", "127.0.0.1", port, HTTPSServerOptions{
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := srv.LeafSPKIPinHex()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	port = listenHTTPSPort(t, srv)

	cli, err := NewHTTPSClientInterfaceWithRetries("https_pin_cli", "127.0.0.1", port, true, 10, HTTPSClientOptions{
		PeerKey:  pin,
		LongPoll: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
}

func TestHTTPSTypeAndMTU(t *testing.T) {
	srv, err := NewHTTPSServerInterface("https_meta", "127.0.0.1", 0, HTTPSServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if srv.GetType() != common.IFTypeHTTPS {
		t.Fatalf("type %v want IFTypeHTTPS", srv.GetType())
	}
	if srv.GetMTU() != DefaultMTU {
		t.Fatalf("MTU %d want %d", srv.GetMTU(), DefaultMTU)
	}
}
