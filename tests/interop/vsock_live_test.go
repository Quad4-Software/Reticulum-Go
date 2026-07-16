// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live Go-Go VSOCK interface interop. Set RUN_LIVE_INTEROP=1.
// Uses AF_VSOCK Local CID for same-host loopback without a guest VM.

//go:build linux && !js

package interop

import (
	"bytes"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdlayher/vsock"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
)

func skipIfVSOCKUnavailable(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/vsock"); err != nil {
		t.Skipf("vsock unavailable: %v", err)
	}
	ln, err := vsock.ListenContextID(vsock.Local, 0, nil)
	if err != nil {
		t.Skipf("vsock Local listen unavailable: %v", err)
	}
	_ = ln.Close()
}

func TestLiveInteropVSOCKGoGoEcho(t *testing.T) {
	liveOrSkip(t)
	skipIfVSOCKUnavailable(t)

	srv, err := interfaces.NewVSOCKServerInterface("live_vsock_srv", 0)
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
		t.Skipf("vsock listen failed: %v", err)
	}
	defer srv.Stop()

	cli, err := interfaces.NewVSOCKClientInterfaceWithRetries("live_vsock_cli", vsock.Local, srv.Port(), true, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("VSOCK client not online")
	}

	payload := []byte{0x56, 0x53, 0x4f, 0x43, 0x4b}
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
		t.Fatal("timeout waiting for live VSOCK packet")
	}
	if !bytes.Equal(got.Load().([]byte), payload) {
		t.Fatalf("got %x want %x", got.Load(), payload)
	}
}

func TestLiveInteropVSOCKBidirectional(t *testing.T) {
	liveOrSkip(t)
	skipIfVSOCKUnavailable(t)

	srv, err := interfaces.NewVSOCKServerInterface("live_vsock_bi_srv", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Skipf("vsock listen failed: %v", err)
	}
	defer srv.Stop()

	cli, err := interfaces.NewVSOCKClientInterfaceWithRetries("live_vsock_bi_cli", vsock.Local, srv.Port(), true, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("VSOCK client not online")
	}

	var fromSrv atomic.Value
	var wg sync.WaitGroup
	wg.Add(1)
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		fromSrv.Store(append([]byte(nil), data...))
		wg.Done()
	})
	time.Sleep(100 * time.Millisecond)
	payload := []byte("from-vsock-server")
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
