// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestRNodeHostPipeRoundTrip(t *testing.T) {
	pipe := NewRNodeHostPipe()
	defer pipe.Close()

	go func() {
		time.Sleep(5 * time.Millisecond)
		_, _ = pipe.PushRX([]byte("from-radio"))
	}()
	buf := make([]byte, 32)
	n, err := pipe.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "from-radio" {
		t.Fatalf("rx got %q", got)
	}

	if _, err := pipe.Write([]byte("to-radio")); err != nil {
		t.Fatal(err)
	}
	if !pipe.WaitTX(time.Second) {
		t.Fatal("expected TX")
	}
	n, err = pipe.PullTX(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "to-radio" {
		t.Fatalf("tx got %q", got)
	}
}

func TestRNodeBLEURIAcceptedAndOpenerUsed(t *testing.T) {
	pipe := NewRNodeHostPipe()
	var sawTarget string
	RegisterRNodePortOpener("ble", func(target string) (SerialPort, error) {
		sawTarget = target
		return pipe, nil
	})
	t.Cleanup(func() { RegisterRNodePortOpener("ble", nil) })

	opts := testRNodeOptions(NewRNodeSim(1))
	opts.Port = "ble://RNode-1"
	r, err := NewRNodeInterface("ble-rnode", true, opts)
	if err != nil {
		t.Fatal(err)
	}
	port, err := r.openPort()
	if err != nil {
		t.Fatal(err)
	}
	if port != pipe {
		t.Fatal("expected registered host pipe")
	}
	if sawTarget != "RNode-1" {
		t.Fatalf("target %q", sawTarget)
	}
	_ = port.Close()
}

func TestRNodeUSBURIRequiresRegistration(t *testing.T) {
	opts := testRNodeOptions(NewRNodeSim(1))
	opts.Port = "usb:///dev/bus/usb/001/002"
	r, err := NewRNodeInterface("usb-rnode", true, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.openPort(); err == nil {
		t.Fatal("expected missing opener error")
	}
}

func TestServeRNodeHostPipeTCP(t *testing.T) {
	pipe := NewRNodeHostPipe()
	addr, stop, err := ServeRNodeHostPipeTCP(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := []byte{0xC0, 0x00, 0x01, 0xC0}
	if _, err := conn.Write(want); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 16)
	_ = pipe // Read blocks until PushRX from relay
	done := make(chan []byte, 1)
	go func() {
		n, err := pipe.Read(buf)
		if err != nil {
			done <- nil
			return
		}
		done <- append([]byte(nil), buf[:n]...)
	}()
	select {
	case got := <-done:
		if !bytes.Equal(got, want) {
			t.Fatalf("pipe rx %x want %x", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for pipe rx")
	}

	if _, err := pipe.Write([]byte{0xAA, 0xBB}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF && n == 0 {
		t.Fatal(err)
	}
	if n < 2 || buf[0] != 0xAA || buf[1] != 0xBB {
		t.Fatalf("tcp got %x n=%d", buf[:n], n)
	}
}
