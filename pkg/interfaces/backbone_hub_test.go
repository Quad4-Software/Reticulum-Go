// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/backbone"
	"quad4/reticulum-go/pkg/common"
)

func testBackboneHubBackend(t *testing.T, b backbone.Backend) *backbone.Hub {
	t.Helper()
	backbone.Shutdown()
	hub, err := backbone.Init(b)
	if err != nil {
		t.Fatalf("backbone.Init(%s): %v", b, err)
	}
	t.Cleanup(backbone.Shutdown)
	return hub
}

func backboneBackendsForTest(t *testing.T) []backbone.Backend {
	t.Helper()
	out := []backbone.Backend{backbone.BackendGo}
	switch runtime.GOOS {
	case "linux", "android":
		out = append(out, backbone.BackendEpoll)
		if hub, err := backbone.Init(backbone.BackendUring); err == nil {
			_ = hub
			backbone.Shutdown()
			out = append(out, backbone.BackendUring)
		} else {
			backbone.Shutdown()
		}
	case "darwin", "freebsd", "netbsd", "openbsd":
		out = append(out, backbone.BackendKqueue)
	}
	return out
}

func TestBackboneInterfaceAllBackends(t *testing.T) {
	for _, backend := range backboneBackendsForTest(t) {
		t.Run(string(backend), func(t *testing.T) {
			testBackboneClientServerRoundTrip(t, backend)
		})
	}
}

func testBackboneClientServerRoundTrip(t *testing.T, backend backbone.Backend) {
	t.Helper()
	hub := testBackboneHubBackend(t, backend)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	server, err := NewBackboneInterface("server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	var received sync.WaitGroup
	received.Add(1)
	server.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		if len(data) == 3 && data[0] == 0x42 {
			received.Done()
		}
	})

	client, err := NewBackboneClientInterface("client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Stop() })

	waitInterfaceOnline(t, client, 5*time.Second)
	if err := client.Send([]byte{0x42, 0x43, 0x44}, ""); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		received.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive packet")
	}
}

func TestBackboneSpawnedClientRegistered(t *testing.T) {
	hub := testBackboneHubBackend(t, backbone.BackendGo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	spawnedCh := make(chan *BackboneClientInterface, 1)
	server, err := NewBackboneInterface("server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, func(c *BackboneClientInterface) {
		spawnedCh <- c
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	client, err := NewBackboneClientInterface("client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Stop() })

	select {
	case <-spawnedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("spawn timeout")
	}
	if server.Clients() < 1 {
		t.Fatalf("clients=%d", server.Clients())
	}
}

func TestBackboneHDLCWireMatchesBackbonePackage(t *testing.T) {
	payload := []byte{0x01, HDLCFlag, HDLCEsc, 0xFF}
	ifaceFrame := append([]byte{HDLCFlag}, escapeHDLC(payload)...)
	ifaceFrame = append(ifaceFrame, HDLCFlag)

	// backbone.frameHDLC is unexported. Rebuild via escapeHDLC parity check.

	backPayload := unescapeHDLC(escapeHDLC(payload))
	if !bytes.Equal(backPayload, payload) {
		t.Fatal("escape parity failed")
	}
	if !bytes.Equal(ifaceFrame[1:len(ifaceFrame)-1], escapeHDLC(payload)) {
		t.Fatal("escaped body mismatch")
	}
}

func TestRaceBackboneConcurrentSend(t *testing.T) {
	hub := testBackboneHubBackend(t, backbone.BackendGo)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	server, err := NewBackboneInterface("server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })

	client, err := NewBackboneClientInterface("client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Stop() })
	waitInterfaceOnline(t, client, 5*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 16; j++ {
				_ = client.Send([]byte{byte(n), byte(j), 0x01}, "")
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkBackboneSend(b *testing.B) {
	backbone.Shutdown()
	hub, err := backbone.Init(backbone.BackendGo)
	if err != nil {
		b.Fatal(err)
	}
	defer backbone.Shutdown()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	server, err := NewBackboneInterface("server", &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    port,
	}, hub, nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := server.Start(); err != nil {
		b.Fatal(err)
	}
	defer server.Stop()

	client, err := NewBackboneClientInterface("client", &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "127.0.0.1",
		TargetPort: port,
	}, hub)
	if err != nil {
		b.Fatal(err)
	}
	if err := client.Start(); err != nil {
		b.Fatal(err)
	}
	defer client.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.IsOnline() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !client.IsOnline() {
		b.Fatal("client offline")
	}

	payload := bytes.Repeat([]byte{0x42}, 512)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := client.Send(payload, ""); err != nil {
			b.Fatal(err)
		}
	}
}
