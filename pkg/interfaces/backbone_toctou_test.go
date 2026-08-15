// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestOracleTOCTOUBackboneAcceptRejectsAfterStop(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: false}
	bi, err := NewBackboneInterface("toctou", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = bi.Stop()

	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	bi.acceptConn(server)
	if bi.Clients() != 0 {
		t.Fatalf("Clients=%d after accept on stopped backbone, want 0", bi.Clients())
	}
}

func TestAdversarialTOCTOUBackboneAcceptVsStop(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: false}
	bi, err := NewBackboneInterface("toctou-race", cfg, hub, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var accepts atomic.Int32
	for range 24 {
		wg.Go(func() {
			s, c := net.Pipe()
			defer func() {
				_ = s.Close()
				_ = c.Close()
			}()
			bi.acceptConn(s)
			accepts.Add(1)
		})
		wg.Go(func() {
			_ = bi.Stop()
		})
	}
	wg.Wait()
	_ = bi.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bi.Clients() == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if bi.Clients() != 0 {
		t.Fatalf("Clients=%d after Stop race, want 0 (TOCTOU admit after stop)", bi.Clients())
	}
	if accepts.Load() == 0 {
		t.Fatal("expected accept attempts")
	}
}

func TestAdversarialTOCTOUBackboneFlapAdmitRecheck(t *testing.T) {
	hub := testBackboneHub(t)
	cfg := &common.InterfaceConfig{Enabled: true, Port: 1, BlockFastFlappingSet: true, BlockFastFlapping: true}
	bi, err := NewBackboneInterface("toctou-flap", cfg, hub, nil)
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
	t.Cleanup(func() { _ = server.Close() })

	ip := peerIP(server)
	bi.recordFastFlap(ip, time.Millisecond)
	if !bi.isFastFlappingBlocked(ip) {
		t.Fatal("expected blocked before accept")
	}
	bi.acceptConn(server)
	if bi.Clients() != 0 {
		t.Fatalf("blocked accept spawned client count=%d", bi.Clients())
	}
}
