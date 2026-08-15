// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build linux && !js

package interfaces

import (
	"bytes"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mdlayher/vsock"

	"quad4/reticulum-go/pkg/common"
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

func TestParseVSOCKContextID(t *testing.T) {
	cid, err := ParseVSOCKContextID(1)
	if err != nil || cid != 1 {
		t.Fatalf("ParseVSOCKContextID(1) = %d, %v", cid, err)
	}
	if _, err := ParseVSOCKContextID(-1); err == nil {
		t.Fatal("expected error for negative context ID")
	}
	if _, err := ParseVSOCKContextID(math.MaxInt32); err != nil {
		t.Fatalf("MaxInt32 should be accepted: %v", err)
	}
}

func TestVSOCKClientServerEcho(t *testing.T) {
	skipIfVSOCKUnavailable(t)

	srv, err := NewVSOCKServerInterface("vsock_srv", 0)
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

	port := srv.Port()
	if port == 0 {
		t.Fatal("expected assigned port")
	}

	cli, err := NewVSOCKClientInterfaceWithRetries("vsock_cli", vsock.Local, port, true, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)

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

func waitVSOCKSessions(t *testing.T, srv *VSOCKServerInterface, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if srv.SessionCount() >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("want %d sessions, have %d", n, srv.SessionCount())
}

func TestVSOCKServerToClientEcho(t *testing.T) {
	skipIfVSOCKUnavailable(t)

	srv, err := NewVSOCKServerInterface("vsock_srv2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Skipf("vsock listen failed: %v", err)
	}
	defer srv.Stop()

	cli, err := NewVSOCKClientInterfaceWithRetries("vsock_cli2", vsock.Local, srv.Port(), true, 20)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	waitOnline(t, cli, 5*time.Second)
	waitVSOCKSessions(t, srv, 1, 5*time.Second)

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

func TestVSOCKInterfaceTypeAndMTU(t *testing.T) {
	cli, err := NewVSOCKClientInterfaceWithRetries("vsock_meta", vsock.Local, 1, false, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Stop()
	if cli.GetType() != common.IFTypeVSOCK {
		t.Fatalf("type = %v, want IFTypeVSOCK", cli.GetType())
	}
	if cli.GetMTU() != DefaultMTU {
		t.Fatalf("MTU = %d, want %d", cli.GetMTU(), DefaultMTU)
	}
	if cli.Bitrate != vsockBitrateGuess {
		t.Fatalf("Bitrate = %d, want %d", cli.Bitrate, vsockBitrateGuess)
	}
}
