// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"fmt"
	"net"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

// Two TCP clients connecting to a spawned-mode server must each get their own
// interface identity and independent tunnel-synthesis state (Python
// spawned_interfaces, RNS 1.6.1).
func TestTCPServerSpawnedClientsIndependentState(t *testing.T) {
	srv, err := NewTCPServerInterface("srv", "127.0.0.1", 0, false, false, false)
	if err != nil {
		t.Fatalf("NewTCPServerInterface: %v", err)
	}
	var spawned []*TCPServerClientInterface
	srv.EnableSpawnedClients(&common.InterfaceConfig{}, func(c *TCPServerClientInterface) {
		spawned = append(spawned, c)
	})
	if srv.AllowsOutgoing() {
		t.Fatal("listener must be receive-only in spawned mode (Python OUT=False)")
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	port := srv.listener.Addr().(*net.TCPAddr).Port
	dial := func() net.Conn {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return conn
	}
	c1 := dial()
	defer c1.Close()
	c2 := dial()
	defer c2.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(spawned) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(spawned) != 2 {
		t.Fatalf("expected 2 spawned clients, got %d", len(spawned))
	}
	if spawned[0] == spawned[1] {
		t.Fatal("each connection must get a distinct interface object")
	}
	if spawned[0].GetName() == spawned[1].GetName() {
		t.Fatal("spawned interfaces need unique names")
	}
	if !spawned[0].TunnelsAllowed() || !spawned[0].LosslessMedium() {
		t.Fatal("spawned client must allow tunnels and report lossless medium")
	}
	spawned[0].SetSynthesisReceived(true)
	if spawned[1].SynthesisReceived() {
		t.Fatal("synthesis state leaked between spawned interfaces")
	}
	if p := spawned[0].SpawnedParent(); p == nil || p.GetName() != "srv" {
		t.Fatal("spawned client must report the listener as parent")
	}
}
