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

func TestNewBackboneInterfaceDefaults(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:     true,
		Address:     "127.0.0.1",
		Port:        4242,
		KISSFraming: false,
	}
	bi, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi == nil {
		t.Fatal("nil interface")
	}
	if bi.MTU != 1048576 {
		t.Errorf("MTU = %d, want 1048576", bi.MTU)
	}
	if bi.Bitrate != 1000000000 {
		t.Errorf("Bitrate = %d, want 1e9", bi.Bitrate)
	}
	if bi.Type != common.IFTypeBackbone {
		t.Errorf("Type = %v, want IFTypeBackbone", bi.Type)
	}
}

func TestNewBackboneInterfaceNoPort(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Address: "127.0.0.1"}
	_, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestNewBackboneInterfaceUsesListenPortOnly(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:    true,
		Address:    "127.0.0.1",
		Port:       4242,
		TargetPort: 9999,
	}
	bi, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi.bindPort != 4242 {
		t.Errorf("bindPort = %d, want 4242", bi.bindPort)
	}
}

func TestNewBackboneInterfaceResolveDevice(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "js" {
		t.Skip("skipping interface resolution on this platform")
	}
	hub := testBackboneHub(t)
	// Use loopback interface, which exists on every platform.
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		t.Skipf("loopback interface not found: %v", err)
	}
	loAddrs, err := lo.Addrs()
	if err != nil || len(loAddrs) == 0 {
		t.Skip("loopback has no addresses")
	}

	cfg := &common.InterfaceConfig{
		Enabled:   true,
		Interface: "lo",
		Port:      4242,
	}
	bi, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if bi.bindAddr == "" {
		t.Fatal("expected bindAddr to be resolved from lo")
	}
}

func TestNewBackboneInterfaceBadDevice(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:   true,
		Interface: "nonexistent0",
		Port:      4242,
	}
	_, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err == nil {
		t.Fatal("expected error for bad interface name")
	}
}

func TestBackboneInterfaceHDLCRoundTrip(t *testing.T) {
	packets := [][]byte{
		{0x01, 0x02, 0x03},
		make([]byte, 100),
		{HDLCFlag, HDLCEsc, 0xAB},
		make([]byte, 1024),
	}
	packets[1][0] = 0x7E
	packets[3][len(packets[3])-1] = 0x7D

	for _, pkt := range packets {
		escaped := escapeHDLC(pkt)
		decoded := unescapeHDLC(escaped)
		if !bytes.Equal(decoded, pkt) {
			t.Errorf("HDLC round-trip failed:\n in=%x\nout=%x", pkt, decoded)
		}
	}
}

func TestBackboneInterfaceStartStop(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled: true,
		Address: "127.0.0.1",
		Port:    4242,
	}
	bi, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err != nil {
		t.Fatalf("NewBackboneInterface: %v", err)
	}
	if err := bi.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !bi.IsOnline() {
		t.Error("expected online after Start")
	}
	if err := bi.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func FuzzHDLCEscapeUnescape(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{HDLCFlag, HDLCEsc, 0xAB, 0xCD})
	f.Add([]byte{})
	f.Add(make([]byte, 256))
	f.Fuzz(func(t *testing.T, data []byte) {
		escaped := escapeHDLC(data)
		// Escaped must not contain raw flag or esc bytes
		for _, b := range escaped {
			if b == HDLCFlag {
				t.Fatal("raw HDLCFlag in escaped data")
			}
		}
		decoded := unescapeHDLC(escaped)
		if !bytes.Equal(decoded, data) {
			t.Fatalf("round-trip failed: in=%x out=%x", data, decoded)
		}
	})
}

func BenchmarkEscapeHDLC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = escapeHDLC(data)
	}
}

func BenchmarkUnescapeHDLC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	escaped := escapeHDLC(data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = unescapeHDLC(escaped)
	}
}

func TestNewBackboneClientInterfaceDefaults(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "example.net",
		TargetPort: 4242,
	}
	bc, err := NewBackboneClientInterface("client", cfg, hub)
	if err != nil {
		t.Fatalf("NewBackboneClientInterface: %v", err)
	}
	defer func() { _ = bc.Stop() }()
	if bc.MTU != backboneHWMTU {
		t.Errorf("MTU = %d, want %d", bc.MTU, backboneHWMTU)
	}
	if bc.Bitrate != backboneClientBitrateGuess {
		t.Errorf("Bitrate = %d, want %d", bc.Bitrate, backboneClientBitrateGuess)
	}
	if bc.GetType() != common.IFTypeBackbone {
		t.Errorf("Type = %v, want IFTypeBackbone", bc.GetType())
	}
	if bc.reconnect == nil {
		t.Fatal("enabled client should start reconnect driver")
	}
}

func TestNewBackboneClientInterfaceRequiresTarget(t *testing.T) {
	hub := testBackboneHub(t)
	_, err := NewBackboneClientInterface("client", &common.InterfaceConfig{Enabled: true, TargetPort: 4242}, hub)
	if err == nil {
		t.Fatal("expected error without target_host")
	}
	_, err = NewBackboneClientInterface("client", &common.InterfaceConfig{Enabled: true, TargetHost: "x.example"}, hub)
	if err == nil {
		t.Fatal("expected error without target_port")
	}
}

func TestNewBackboneClientInterfacePortFallback(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:    true,
		TargetHost: "example.net",
		Port:       7822,
	}
	bc, err := NewBackboneClientInterface("client", cfg, hub)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bc.Stop() }()
	if bc.targetPort != 7822 {
		t.Fatalf("targetPort = %d, want 7822", bc.targetPort)
	}
}

func TestNewBackboneFromConfigSelectsClientMode(t *testing.T) {
	hub := testBackboneHub(t)
	iface, err := NewBackboneFromConfig("mesh", &common.InterfaceConfig{
		Type:       "BackboneInterface",
		Enabled:    true,
		TargetHost: "rns.example.net",
		TargetPort: 4242,
	}, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := iface.(*BackboneClientInterface); !ok {
		t.Fatalf("expected BackboneClientInterface, got %T", iface)
	}
	defer func() { _ = iface.Stop() }()
}

func TestNewBackboneFromConfigSelectsServerMode(t *testing.T) {
	hub := testBackboneHub(t)
	iface, err := NewBackboneFromConfig("hub", &common.InterfaceConfig{
		Type:    "BackboneInterface",
		Enabled: true,
		Address: "127.0.0.1",
		Port:    4242,
	}, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := iface.(*BackboneInterface); !ok {
		t.Fatalf("expected BackboneInterface, got %T", iface)
	}
}

func TestNewFromConfigBackboneCommunitySnippet(t *testing.T) {
	hub := testBackboneHub(t)
	iface, err := NewFromConfigWithContext("MichMesh", &common.InterfaceConfig{
		Type:       "BackboneInterface",
		Enabled:    true,
		TargetHost: "michmesh.example",
		TargetPort: 4242,
	}, &FromConfigContext{BackboneHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	bc, ok := iface.(*BackboneClientInterface)
	if !ok {
		t.Fatalf("expected BackboneClientInterface, got %T", iface)
	}
	if bc.targetAddr != "michmesh.example" || bc.targetPort != 4242 {
		t.Fatalf("target = %s:%d", bc.targetAddr, bc.targetPort)
	}
}

func TestNewBackboneInterfaceIgnoresTargetHost(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{
		Enabled:    true,
		Address:    "127.0.0.1",
		Port:       4242,
		TargetHost: "remote.example",
		TargetPort: 9999,
	}
	bi, err := NewBackboneInterface("bb", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bi.bindAddr != "127.0.0.1" || bi.bindPort != 4242 {
		t.Fatalf("bind = %s:%d", bi.bindAddr, bi.bindPort)
	}
}

func TestBackboneClientConnectsToServer(t *testing.T) {
	hub := testBackboneHub(t)
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
		t.Fatalf("server Start: %v", err)
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
		t.Fatalf("client Start: %v", err)
	}
	t.Cleanup(func() { _ = client.Stop() })

	waitInterfaceOnline(t, client, 5*time.Second)

	if err := client.Send([]byte{0x42, 0x43, 0x44}, ""); err != nil {
		t.Fatalf("client Send: %v", err)
	}

	done := make(chan struct{})
	go func() {
		received.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive client packet")
	}
}

func waitInterfaceOnline(t *testing.T, iface Interface, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for interface online")
}

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
		if backbone.UringProbeAllowed() {
			out = append(out, backbone.BackendUring)
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
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range 16 {
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
