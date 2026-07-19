// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestBackboneFastFlappingBlocksAfterGrace(t *testing.T) {
	hub := testBackboneHub(t)

	cfg := &common.InterfaceConfig{
		Enabled:                  true,
		Port:                     1,
		BlockFastFlapping:        true,
		BlockFastFlappingSet:     true,
		FastFlappingThreshold:    30,
		FastFlappingGrace:        2,
		FastFlappingBlockTimeMin: 60,
	}
	bi, err := NewBackboneInterface("flap", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapThreshold = 30 * time.Second
	bi.fastFlapGrace = 2

	ip := "203.0.113.50"
	bi.recordFastFlap(ip, time.Second)
	bi.recordFastFlap(ip, time.Second)
	if bi.isFastFlappingBlocked(ip) {
		t.Fatal("should not block at grace boundary")
	}
	bi.recordFastFlap(ip, time.Second)
	if !bi.isFastFlappingBlocked(ip) {
		t.Fatal("expected block after exceeding grace")
	}
	if bi.BlockedIPCount() != 1 {
		t.Fatalf("BlockedIPCount=%d want 1", bi.BlockedIPCount())
	}
}

func TestBackboneFastFlappingIgnoresLongLived(t *testing.T) {
	hub := testBackboneHub(t)

	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
	bi, err := NewBackboneInterface("flap", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapThreshold = 5 * time.Second
	bi.fastFlapGrace = 0

	ip := "203.0.113.51"
	bi.recordFastFlap(ip, 10*time.Second)
	if bi.isFastFlappingBlocked(ip) {
		t.Fatal("long-lived connections must not count as flaps")
	}
}

func TestBackboneAcceptConnRejectsBlockedIP(t *testing.T) {
	hub := testBackboneHub(t)

	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
	bi, err := NewBackboneInterface("flap", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	bi.fastFlapGrace = 0
	bi.fastFlapThreshold = time.Minute

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	server, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	ip := peerIP(server)
	bi.recordFastFlap(ip, time.Millisecond)
	if !bi.isFastFlappingBlocked(ip) {
		t.Fatal("fixture IP should be blocked")
	}
	bi.acceptConn(server)
	if bi.Clients() != 0 {
		t.Fatalf("blocked accept should not spawn client, got %d", bi.Clients())
	}
}
