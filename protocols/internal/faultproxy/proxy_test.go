// SPDX-License-Identifier: LicenseRef-Reticulum
package faultproxy

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func echoServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, _ = io.Copy(c, c)
				_ = c.Close()
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func frame(payload []byte) []byte {
	out := []byte{hdlcFlag}
	for _, b := range payload {
		if b == hdlcFlag || b == hdlcEscape {
			out = append(out, hdlcEscape, b^hdlcEscXor)
		} else {
			out = append(out, b)
		}
	}
	return append(out, hdlcFlag)
}

func exchange(t *testing.T, addr string, send []byte, want int, timeout time.Duration) []byte {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write(send); err != nil {
		t.Fatal(err)
	}
	out := make([]byte, want)
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	_, err = io.ReadFull(c, out)
	if err != nil {
		t.Fatalf("read %d bytes: %v (got %d)", want, err, len(out))
	}
	return out
}

// With all faults off the proxy is byte-transparent.
func TestProxyTransparentWhenNoFaults(t *testing.T) {
	target, stop := echoServer(t)
	defer stop()
	p, err := New(target, 1, Faults{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	var payload []byte
	for i := 0; i < 50; i++ {
		payload = append(payload, frame([]byte{byte(i), 0x7e, 0x7d, byte(i * 3)})...)
	}
	got := exchange(t, p.Addr, payload, len(payload), 3*time.Second)
	if !bytes.Equal(got, payload) {
		t.Fatalf("not transparent: sent %d got %d bytes, equal=%v", len(payload), len(got), bytes.Equal(got, payload))
	}
}

// Seeded determinism: two proxies with the same seed produce the same fault decisions.
func TestProxySeededDeterminism(t *testing.T) {
	target, stop := echoServer(t)
	defer stop()
	run := func() Stats {
		p, err := New(target, 42, Faults{Drop: 0.3, Duplicate: 0.3, Reorder: 0.3})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()
		var payload []byte
		for i := 0; i < 100; i++ {
			payload = append(payload, frame([]byte{byte(i), byte(i + 1), byte(i + 2)})...)
		}
		c, err := net.DialTimeout("tcp", p.Addr, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = c.Write(payload)
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = io.Copy(io.Discard, c)
		_ = c.Close()
		return p.Snapshot()
	}
	s1 := run()
	s2 := run()
	if s1.Dropped != s2.Dropped || s1.Duplicated != s2.Duplicated || s1.Reordered != s2.Reordered {
		t.Errorf("non-deterministic: %+v vs %+v", s1, s2)
	}
	if s1.Dropped == 0 {
		t.Error("seed 42 produced no drops over 100 frames — check RNG wiring")
	}
}

// With duplication but no drops/reordering, every frame arrives at least once.
func TestProxyDuplicationNeverLoses(t *testing.T) {
	target, stop := echoServer(t)
	defer stop()
	p, err := New(target, 7, Faults{Duplicate: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	var payload []byte
	count := 30
	for i := 0; i < count; i++ {
		payload = append(payload, frame([]byte{byte(i)})...)
	}
	c, err := net.DialTimeout("tcp", p.Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	echoed, _ := io.ReadAll(c) // reads until deadline; duplicates make it longer
	_ = c.Close()
	if len(echoed) < len(payload) {
		t.Fatalf("echoed %d bytes < sent %d with duplication-only faults", len(echoed), len(payload))
	}
	// every original frame must appear in order in the echoed stream
	pos := 0
	for i := 0; i < count; i++ {
		needle := frame([]byte{byte(i)})
		idx := bytes.Index(echoed[pos:], needle)
		if idx < 0 {
			t.Fatalf("frame %d missing from echoed stream", i)
		}
		pos += idx + len(needle)
	}
	s := p.Snapshot()
	if s.FramesOut < s.FramesIn {
		t.Errorf("frames out %d < frames in %d with duplication-only faults", s.FramesOut, s.FramesIn)
	}
	if s.Duplicated == 0 {
		t.Error("no duplicates injected over 30 frames at p=0.5")
	}
}

// Byte mode corrupts the stream when enabled.
func TestProxyByteModeCorrupts(t *testing.T) {
	target, stop := echoServer(t)
	defer stop()
	p, err := New(target, 3, Faults{ByteMode: true, Corrupt: 1.0})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	payload := bytes.Repeat([]byte{0x41}, 2000)
	got := exchange(t, p.Addr, payload, len(payload), 3*time.Second)
	if bytes.Equal(got, payload) {
		t.Error("corruption at p=1.0 left stream unchanged")
	}
}

// Flap resets active connections.
func TestProxyFlap(t *testing.T) {
	target, stop := echoServer(t)
	defer stop()
	p, err := New(target, 1, Faults{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	c, err := net.DialTimeout("tcp", p.Addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write(frame([]byte{1})); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	p.Flap()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	one := make([]byte, 1)
	for {
		_, err := c.Read(one)
		if err != nil {
			return // connection died as expected
		}
	}
}
