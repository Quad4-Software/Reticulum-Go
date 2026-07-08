// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package interfaces

import (
	"net"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

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
	if bc.MTU != backboneHWMTU {
		t.Errorf("MTU = %d, want %d", bc.MTU, backboneHWMTU)
	}
	if bc.Bitrate != backboneClientBitrateGuess {
		t.Errorf("Bitrate = %d, want %d", bc.Bitrate, backboneClientBitrateGuess)
	}
	if bc.GetType() != common.IFTypeBackbone {
		t.Errorf("Type = %v, want IFTypeBackbone", bc.GetType())
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
