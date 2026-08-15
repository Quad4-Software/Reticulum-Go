// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"bytes"
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestAppendFrameKISSMatchesPythonShape(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x02}
	got := appendFrameKISS(nil, payload)
	want := []byte{KISSFend, KISSCmdData, 0x00, 0x01, 0x02, KISSFend}
	if !bytes.Equal(got, want) {
		t.Fatalf("appendFrameKISS=%x want %x", got, want)
	}
}

func TestAppendFrameKISSEscapesSpecials(t *testing.T) {
	payload := []byte{KISSFend, KISSFesc, 0x42}
	got := appendFrameKISS(nil, payload)
	want := []byte{
		KISSFend, KISSCmdData,
		KISSFesc, KISSTFend,
		KISSFesc, KISSTFesc,
		0x42,
		KISSFend,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("appendFrameKISS=%x want %x", got, want)
	}
}

func TestKISSStreamDecoderRoundTrip(t *testing.T) {
	payload := []byte{0x10, KISSFend, 0x20, KISSFesc, 0x30}
	frame := appendFrameKISS(nil, payload)
	var got []byte
	d := newKISSStreamDecoder(DefaultMTU, func(p []byte) {
		got = append([]byte(nil), p...)
	})
	d.feed(frame)
	if !bytes.Equal(got, payload) {
		t.Fatalf("decoded=%x want %x", got, payload)
	}
}

func TestKISSStreamDecoderIgnoresNonDataCommand(t *testing.T) {
	frame := []byte{KISSFend, 0x01, 0xaa, 0xbb, KISSFend}
	called := false
	d := newKISSStreamDecoder(DefaultMTU, func([]byte) { called = true })
	d.feed(frame)
	if called {
		t.Fatal("non-CMD_DATA frame must not be delivered")
	}
}

func TestTCPClientProcessOutgoingUsesKISSWhenEnabled(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	tc := &TCPClientInterface{
		BaseInterface: NewBaseInterface("kiss-client", common.IFTypeTCP, true),
		conn:          client,
		kissFraming:   true,
		txFrame:       make([]byte, 0, 64),
	}
	tc.Online = true
	tc.MTU = DefaultMTU

	payload := []byte{0x00, 0x01, 0x02}
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		done <- append([]byte(nil), buf[:n]...)
	}()

	if err := tc.ProcessOutgoing(payload); err != nil {
		t.Fatalf("ProcessOutgoing: %v", err)
	}
	var got []byte
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for framed write")
	}
	want := appendFrameKISS(nil, payload)
	if !bytes.Equal(got, want) {
		t.Fatalf("wire=%x want KISS %x (got HDLC-like if starts with 7e)", got, want)
	}
}

func TestTCPClientProcessOutgoingUsesHDLCByDefault(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	tc := &TCPClientInterface{
		BaseInterface: NewBaseInterface("hdlc-client", common.IFTypeTCP, true),
		conn:          client,
		kissFraming:   false,
		txFrame:       make([]byte, 0, 64),
	}
	tc.Online = true
	tc.MTU = DefaultMTU

	payload := []byte{0x00, 0x01, 0x02}
	done := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		done <- append([]byte(nil), buf[:n]...)
	}()

	if err := tc.ProcessOutgoing(payload); err != nil {
		t.Fatalf("ProcessOutgoing: %v", err)
	}
	var got []byte
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for framed write")
	}
	want := appendFrameHDLC(nil, payload)
	if !bytes.Equal(got, want) {
		t.Fatalf("wire=%x want HDLC %x", got, want)
	}
}

// FuzzKISSStreamDecoderRoundTrip encodes arbitrary payloads and requires the
// decoder to recover them exactly.
func FuzzKISSStreamDecoderRoundTrip(f *testing.F) {
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte{KISSFend, KISSFesc})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 512 {
			t.Skip()
		}
		frame := appendFrameKISS(nil, payload)
		var got []byte
		d := newKISSStreamDecoder(1024, func(p []byte) {
			got = append([]byte(nil), p...)
		})
		d.feed(frame)
		if len(payload) == 0 {
			if len(got) != 0 {
				t.Fatalf("empty payload delivered %x", got)
			}
			return
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("roundtrip mismatch got=%x want=%x", got, payload)
		}
	})
}
