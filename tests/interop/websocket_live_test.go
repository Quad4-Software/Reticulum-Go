// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live WebSocket interface echo against a minimal in-test RFC 6455 server.
// The interface is a Go extension, so this is a Go-Go check.
// Set RUN_LIVE_INTEROP=1 to enable.

//go:build !js

package interop

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
)

const wsAcceptGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsEchoServer is a minimal RFC 6455 echo endpoint: handshake then echo each
// binary frame payload back unmasked.
type wsEchoServer struct {
	ln net.Listener
}

func startWSEchoServer(t *testing.T) *wsEchoServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ws listen: %v", err)
	}
	s := &wsEchoServer{ln: ln}
	go s.serve()
	return s
}

func (s *wsEchoServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *wsEchoServer) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return
	}
	sum := sha1.Sum([]byte(key + wsAcceptGUID))
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		return
	}
	for {
		payload, opcode, err := wsReadFrame(br)
		if err != nil {
			return
		}
		switch opcode {
		case 0x8: // close
			_ = wsWriteFrame(conn, payload, 0x8)
			return
		case 0x9: // ping
			_ = wsWriteFrame(conn, payload, 0xA)
		case 0x1, 0x2, 0x0: // text/binary/continuation
			if err := wsWriteFrame(conn, payload, 0x2); err != nil {
				return
			}
		}
	}
}

// wsReadFrame reads one client frame and returns the unmasked payload.
func wsReadFrame(br *bufio.Reader) ([]byte, byte, error) {
	hdr := make([]byte, 2)
	if _, err := readFullWS(br, hdr); err != nil {
		return nil, 0, err
	}
	opcode := hdr[0] & 0x0f
	masked := hdr[1]&0x80 != 0
	plen := uint64(hdr[1] & 0x7f)
	if plen == 126 {
		var b [2]byte
		if _, err := readFullWS(br, b[:]); err != nil {
			return nil, 0, err
		}
		plen = uint64(binary.BigEndian.Uint16(b[:]))
	} else if plen == 127 {
		var b [8]byte
		if _, err := readFullWS(br, b[:]); err != nil {
			return nil, 0, err
		}
		plen = binary.BigEndian.Uint64(b[:])
	}
	if plen > 1<<22 {
		return nil, 0, errString("ws frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := readFullWS(br, mask[:]); err != nil {
			return nil, 0, err
		}
	}
	payload := make([]byte, plen)
	if _, err := readFullWS(br, payload); err != nil {
		return nil, 0, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, opcode, nil
}

func readFullWS(br *bufio.Reader, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := br.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func wsWriteFrame(conn net.Conn, payload []byte, opcode byte) error {
	hdr := []byte{0x80 | opcode}
	switch {
	case len(payload) < 126:
		hdr = append(hdr, byte(len(payload)))
	case len(payload) <= 0xffff:
		hdr = append(hdr, 126, byte(len(payload)>>8), byte(len(payload)))
	default:
		hdr = append(hdr, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(len(payload)))
		hdr = append(hdr, b[:]...)
	}
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// TestLiveInteropWebSocketGoEcho drives the Go WebSocketInterface client
// against the in-test RFC 6455 echo server and back.
func TestLiveInteropWebSocketGoEcho(t *testing.T) {
	liveOrSkip(t)

	srv := startWSEchoServer(t)
	defer srv.ln.Close()
	url := "ws://" + srv.ln.Addr().String() + "/rns"

	cli, err := interfaces.NewWebSocketInterface("live_ws", url, true)
	if err != nil {
		t.Fatalf("NewWebSocketInterface: %v", err)
	}
	defer cli.Stop()
	if err := cli.Start(); err != nil {
		t.Fatalf("start ws client: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !cli.IsOnline() {
		time.Sleep(50 * time.Millisecond)
	}
	if !cli.IsOnline() {
		t.Fatal("websocket client not online")
	}

	var got atomic.Value
	done := make(chan struct{})
	cli.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Store(append([]byte(nil), data...))
		close(done)
	})

	// First byte must keep bit 7 clear: the inbound IFAC check treats it as a
	// masked-packet flag and drops flagged packets when no IFAC is set.
	payload := []byte{0x11, 0xad, 0xbe, 0xef, 0x7e, 0x7d}
	if err := cli.Send(payload, ""); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("no echo from ws server")
	}
	recv, _ := got.Load().([]byte)
	if string(recv) != string(payload) {
		t.Fatalf("echo = %x, want %x", recv, payload)
	}
}
