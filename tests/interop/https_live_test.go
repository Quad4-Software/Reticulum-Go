// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Go-Go HTTPS long-poll interface interop. Set RUN_LIVE_INTEROP=1.

//go:build !js

package interop

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func TestLiveInteropHTTPSGoGoEcho(t *testing.T) {
	liveOrSkip(t)

	port := freeTCPPort(t)
	srv, err := interfaces.NewHTTPSServerInterface("live_https_srv", "127.0.0.1", port, interfaces.HTTPSServerOptions{
		Path:     "/rns",
		LongPoll: 2 * time.Second,
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

	cli, err := interfaces.NewHTTPSClientInterfaceWithRetries("live_https_cli", "127.0.0.1", port, true, -1, interfaces.HTTPSClientOptions{
		PeerKey:  pin,
		Path:     "/rns",
		LongPoll: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("HTTPS client not online")
	}

	payload := []byte{0x48, 0x54, 0x54, 0x50, 0x53}
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for live HTTPS packet")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("got %x want %x", got.Load(), payload)
	}
}

func TestLiveInteropHTTPSBidirectional(t *testing.T) {
	liveOrSkip(t)

	port := freeTCPPort(t)
	srv, err := interfaces.NewHTTPSServerInterface("live_https_bi_srv", "127.0.0.1", port, interfaces.HTTPSServerOptions{
		LongPoll: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli, err := interfaces.NewHTTPSClientInterfaceWithRetries("live_https_bi_cli", "127.0.0.1", port, true, -1, interfaces.HTTPSClientOptions{
		LongPoll: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("HTTPS client not online")
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && srv.PeerCount() < 1 {
		time.Sleep(50 * time.Millisecond)
	}
	if srv.PeerCount() < 1 {
		t.Fatal("no HTTPS peers registered")
	}

	var fromSrv atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		fromSrv.Store(append([]byte(nil), data...))
		wg.Done()
	})
	time.Sleep(50 * time.Millisecond)
	payload := []byte("from-https-server")
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
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server-to-client HTTPS packet")
	}
	if !bytes.Equal(fromSrv.Load().([]byte), payload) {
		t.Fatalf("got %x want %x", fromSrv.Load(), payload)
	}
}
