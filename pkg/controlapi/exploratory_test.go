// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/identity"
)

// FuzzWSReadMessageExploratory feeds masked client frames into readMessage.
// Oversize lengths must error, and successful payloads must stay within
// wsMaxMessageBytes.
func FuzzWSReadMessageExploratory(f *testing.F) {
	f.Add(maskedTextFrame([]byte(`{"type":"subscribe_announces"}`)))
	f.Add(maskedTextFrame([]byte(`{}`)))
	f.Add([]byte{0x81, 0x80})
	f.Add([]byte{0x81, 0xfe, 0xff, 0xff})
	f.Add([]byte{0x81, 0xff, 0, 0, 0, 0, 0, 0, 0, 1})
	f.Add([]byte{0x89, 0x80, 0, 0, 0, 0})
	f.Add([]byte{0x88, 0x80, 0, 0, 0, 0})
	f.Add([]byte{0x01, 0x81, 0, 0, 0, 0, 'x'})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1<<16 {
			t.Skip()
		}
		conn := &wsConn{conn: discardConn{}, reader: bufio.NewReader(bytes.NewReader(raw))}
		payload, err := conn.readMessage()
		if err != nil {
			return
		}
		if len(payload) > wsMaxMessageBytes {
			t.Fatalf("payload len=%d exceeds cap", len(payload))
		}
	})
}

func TestWSOutboxDropWhenFull(t *testing.T) {
	srv, _ := newTestServer(t)
	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession("exploratory-outbox", ident)
	c := newWSClient(srv, sess, &wsConn{conn: discardConn{}, reader: bufio.NewReader(bytes.NewReader(nil))})

	for i := range wsClientOutboxSize {
		c.send(map[string]any{"type": "fill", "i": i})
	}
	c.send(map[string]any{"type": "overflow"})
	if len(c.outbox) != wsClientOutboxSize {
		t.Fatalf("outbox len=%d want %d after drop", len(c.outbox), wsClientOutboxSize)
	}
}

func TestWSWriteLoopDoneBeforeEnable(t *testing.T) {
	srv, _ := newTestServer(t)
	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession("exploratory-done", ident)
	rc := newRecordConn()
	c := newWSClient(srv, sess, &wsConn{conn: rc, reader: bufio.NewReader(rc)})
	c.startWriter()
	payload := []byte(`{"type":"should-not-flush"}`)
	select {
	case c.outbox <- payload:
	default:
		t.Fatal("outbox full")
	}
	c.close()
	time.Sleep(20 * time.Millisecond)
	got := rc.snapshot()
	if bytes.Contains(got, payload) {
		t.Fatalf("queued payload flushed after close before enableWrites: %q", got)
	}
}

func maskedTextFrame(payload []byte) []byte {
	mask := [4]byte{1, 2, 3, 4}
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	var hdr []byte
	switch {
	case len(payload) < 126:
		hdr = []byte{0x81, byte(0x80 | len(payload))}
	default:
		hdr = make([]byte, 4)
		hdr[0] = 0x81
		hdr[1] = 0x80 | 126
		binary.BigEndian.PutUint16(hdr[2:], uint16(len(payload)))
	}
	out := append(hdr, mask[:]...)
	return append(out, masked...)
}

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(b []byte) (int, error)      { return len(b), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return discardAddr("local") }
func (discardConn) RemoteAddr() net.Addr             { return discardAddr("remote") }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

type discardAddr string

func (a discardAddr) Network() string { return "discard" }
func (a discardAddr) String() string  { return string(a) }
