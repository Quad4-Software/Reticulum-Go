// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

type udpJunkCounter struct {
	conn  *net.UDPConn
	junk  atomic.Int32
	other atomic.Int32
	done  chan struct{}
}

func newUDPJunkCounter(t *testing.T) *udpJunkCounter {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	c := &udpJunkCounter{conn: conn, done: make(chan struct{})}
	buf := make([]byte, 2048)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			n, err := conn.Read(buf)
			select {
			case <-c.done:
				return
			default:
			}
			if err != nil {
				continue
			}
			if n == 1 && buf[0] == 0x00 {
				c.junk.Add(1)
			} else {
				c.other.Add(1)
			}
		}
	}()
	t.Cleanup(func() {
		close(c.done)
		_ = conn.Close()
	})
	return c
}

func TestUDPKeepaliveSendsHoldOpen(t *testing.T) {
	recv := newUDPJunkCounter(t)
	ui, err := NewUDPInterface("ka", "127.0.0.1:0", recv.conn.LocalAddr().String(), true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	ui.SetKeepaliveInterval(60 * time.Millisecond)
	if err := ui.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ui.Stop() }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && recv.junk.Load() < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if recv.junk.Load() < 2 {
		t.Fatalf("expected periodic hold-open datagrams, got %d", recv.junk.Load())
	}
}

func TestUDPKeepaliveDisabledByDefault(t *testing.T) {
	recv := newUDPJunkCounter(t)
	ui, err := NewUDPInterface("ka-off", "127.0.0.1:0", recv.conn.LocalAddr().String(), true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	if err := ui.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ui.Stop() }()

	time.Sleep(250 * time.Millisecond)
	if n := recv.junk.Load() + recv.other.Load(); n != 0 {
		t.Fatalf("unexpected datagrams with keepalive disabled: %d", n)
	}
}

// TestUDPKeepaliveSuppressedByTraffic mirrors WireGuard semantics: the
// hold-open datagram is only sent when the link is idle.
func TestUDPKeepaliveSuppressedByTraffic(t *testing.T) {
	recv := newUDPJunkCounter(t)
	ui, err := NewUDPInterface("ka-busy", "127.0.0.1:0", recv.conn.LocalAddr().String(), true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	ui.SetKeepaliveInterval(100 * time.Millisecond)
	if err := ui.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ui.Stop() }()

	payload := []byte{0x01, 0x02, 0x03}
	until := time.Now().Add(350 * time.Millisecond)
	for time.Now().Before(until) {
		if err := ui.Send(payload, ""); err != nil {
			t.Fatalf("send: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := recv.junk.Load(); n != 0 {
		t.Fatalf("keepalive fired while link was busy: %d junk datagrams", n)
	}
}

// TestUDPKeepaliveRemoteDropSafety verifies the hold-open byte fails packet
// decode so Python and Go peers drop it without state changes.
func TestUDPKeepaliveRemoteDropSafety(t *testing.T) {
	if len(udpKeepalivePayload) != 1 {
		t.Fatalf("keepalive payload len=%d want 1", len(udpKeepalivePayload))
	}
	p := &packet.Packet{Raw: udpKeepalivePayload}
	if err := p.Unpack(); err == nil {
		t.Fatal("keepalive payload decoded as a valid packet")
	}
}
