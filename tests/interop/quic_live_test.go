// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

// Live Go-Go QUIC interface interop. Set RUN_LIVE_INTEROP=1.

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

func TestLiveInteropQUICGoGoEcho(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	srv, err := interfaces.NewQUICServerInterface("live_quic_srv", "127.0.0.1", port, interfaces.QUICServerOptions{})
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

	cli, err := interfaces.NewQUICClientInterfaceWithRetries("live_quic_cli", "127.0.0.1", port, true, -1, interfaces.QUICClientOptions{
		PeerKey: pin,
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
		t.Fatal("QUIC client not online")
	}

	payload := []byte{0x51, 0x55, 0x49, 0x43}
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
		t.Fatal("timeout waiting for live QUIC packet")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("got %x want %x", got.Load(), payload)
	}
}

func TestLiveInteropQUICBidirectional(t *testing.T) {
	liveOrSkip(t)

	port := freeUDPPort(t)
	srv, err := interfaces.NewQUICServerInterface("live_quic_bi_srv", "127.0.0.1", port, interfaces.QUICServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	cli, err := interfaces.NewQUICClientInterfaceWithRetries("live_quic_bi_cli", "127.0.0.1", port, true, -1, interfaces.QUICClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("QUIC client not online")
	}

	var fromSrv atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		fromSrv.Store(append([]byte(nil), data...))
		wg.Done()
	})
	time.Sleep(100 * time.Millisecond)
	payload := []byte("from-server")
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
		t.Fatal("timeout waiting for server-to-client packet")
	}
	if !bytes.Equal(fromSrv.Load().([]byte), payload) {
		t.Fatalf("got %q", fromSrv.Load())
	}
}
