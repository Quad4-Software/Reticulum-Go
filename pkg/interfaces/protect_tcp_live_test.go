// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/protect"
)

// TestLiveTCPConnCapPrevent sheds excess real TCP accepts.
func TestLiveTCPConnCapPrevent(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxConns:     2,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv, err := NewTCPServerInterface("tcp_protect_live", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for i := range 2 {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("dial hold %d: %v", i, err)
		}
		held = append(held, c)
	}
	time.Sleep(80 * time.Millisecond)

	rejected := 0
	for range 8 {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			continue
		}
		// Server closes over-cap accepts immediately. A short read should EOF.
		_ = c.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		one := make([]byte, 1)
		_, rerr := c.Read(one)
		_ = c.Close()
		if rerr != nil {
			rejected++
		}
	}
	time.Sleep(100 * time.Millisecond)
	if e.TripCount(protect.ReasonConn) == 0 {
		t.Fatalf("expected conn trips under live accept storm rejected=%d warn=%q", rejected, buf.String())
	}
}

// TestLiveTCPDetectConnStormAllowsAccepts keeps IDS non-blocking on real accepts.
func TestLiveTCPDetectConnStormAllowsAccepts(t *testing.T) {
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModeDetect,
		MaxConns:     1,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv, err := NewTCPServerInterface("tcp_detect_live", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	var held []net.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	var accepted atomic.Int64
	for range 4 {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		held = append(held, c)
		accepted.Add(1)
	}
	time.Sleep(150 * time.Millisecond)
	if accepted.Load() < 4 {
		t.Fatalf("detect should allow dials got=%d", accepted.Load())
	}
	if e.TripCount(protect.ReasonConn) == 0 {
		t.Fatal("detect should trip conn over cap")
	}
}
