// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bufio"
	"crypto/sha1" // #nosec G505 -- RFC 6455 mandates SHA-1 for the handshake accept key
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sync"

	"quad4/reticulum-go/pkg/debug"
)

// RFC 6455 framing constants. A minimal server-side implementation lives
// here rather than pulling in a WebSocket dependency: the control API only
// ever exchanges small, unfragmented JSON text frames.
const (
	wsGUID            = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsOpcodeText      = 0x1
	wsOpcodeBinary    = 0x2
	wsOpcodeClose     = 0x8
	wsOpcodePing      = 0x9
	wsOpcodePong      = 0xA
	wsFinBit          = 0x80
	wsOpcodeMask      = 0x0F
	wsMaskedBit       = 0x80
	wsLenMask         = 0x7F
	wsLen16           = 126
	wsLen64           = 127
	wsMaxMessageBytes = 1 << 20 // 1 MiB cap on control-plane JSON payloads
)

// wsConn is a hijacked HTTP connection speaking the WebSocket frame format.
// Clients (the control API's callers) must mask their frames per RFC 6455.
// the server never masks outbound frames.
type wsConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

// wsPendingUpgrade holds a hijacked connection whose 101 response has not
// been flushed yet. Callers register the session client before Flush so the
// peer cannot observe a ready WebSocket before broadcast delivery works.
type wsPendingUpgrade struct {
	conn   net.Conn
	rw     *bufio.ReadWriter
	accept string
}

// beginWebSocketUpgrade hijacks w without sending the handshake response.
// Callers must have already verified the Upgrade and Sec-WebSocket-Key
// headers so earlier failures can still use a normal HTTP error response.
func beginWebSocketUpgrade(w http.ResponseWriter, r *http.Request) (*wsPendingUpgrade, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("controlapi: response writer does not support hijacking")
	}
	key := r.Header.Get("Sec-WebSocket-Key")

	netConn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	return &wsPendingUpgrade{
		conn:   netConn,
		rw:     rw,
		accept: computeWSAccept(key),
	}, nil
}

// Conn returns a wsConn bound to the hijacked socket. Safe to use for
// session registration before Flush: outbound frames must wait until after
// Flush so they cannot interleave with the HTTP 101 response.
func (u *wsPendingUpgrade) Conn() *wsConn {
	return &wsConn{conn: u.conn, reader: u.rw.Reader}
}

// Flush writes the WebSocket handshake response. On failure the underlying
// connection is closed.
func (u *wsPendingUpgrade) Flush() error {
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + u.accept + "\r\n\r\n"
	if _, err := u.rw.Write([]byte(response)); err != nil {
		_ = u.conn.Close()
		return err
	}
	if err := u.rw.Flush(); err != nil {
		_ = u.conn.Close()
		return err
	}
	return nil
}

func computeWSAccept(key string) string {
	h := sha1.New() // #nosec G401 -- RFC 6455 mandates SHA-1 for the handshake accept key
	h.Write([]byte(key))
	h.Write([]byte(wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// readMessage returns the payload of the next text or binary frame,
// answering pings and discarding pongs internally. Fragmented messages are
// rejected: control-plane commands always fit in a single frame.
func (c *wsConn) readMessage() ([]byte, error) {
	for {
		header := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, header); err != nil {
			return nil, err
		}
		fin := header[0]&wsFinBit != 0
		opcode := header[0] & wsOpcodeMask
		masked := header[1]&wsMaskedBit != 0
		length := int(header[1] & wsLenMask)
		// RFC 6455 section 5.1: client-to-server frames must be masked.
		if !masked {
			return nil, errors.New("controlapi: client frames must be masked")
		}

		switch length {
		case wsLen16:
			ext := make([]byte, 2)
			if _, err := io.ReadFull(c.reader, ext); err != nil {
				return nil, err
			}
			length = int(binary.BigEndian.Uint16(ext))
		case wsLen64:
			ext := make([]byte, 8)
			if _, err := io.ReadFull(c.reader, ext); err != nil {
				return nil, err
			}
			v := binary.BigEndian.Uint64(ext)
			if v > uint64(wsMaxMessageBytes) {
				return nil, fmt.Errorf("controlapi: websocket message too large: %d bytes", v)
			}
			length = int(v)
		}
		if length > wsMaxMessageBytes {
			return nil, fmt.Errorf("controlapi: websocket message too large: %d bytes", length)
		}

		var maskKey [4]byte
		if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
			return nil, err
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			return nil, err
		}
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}

		switch opcode {
		case wsOpcodeClose:
			return nil, io.EOF
		case wsOpcodePing:
			if err := c.writeFrame(wsOpcodePong, payload); err != nil {
				return nil, err
			}
			continue
		case wsOpcodePong:
			continue
		case wsOpcodeText, wsOpcodeBinary:
			if !fin {
				return nil, errors.New("controlapi: fragmented websocket messages are not supported")
			}
			return payload, nil
		default:
			return nil, fmt.Errorf("controlapi: unsupported websocket opcode: %d", opcode)
		}
	}
}

// writeMessage sends payload as a single unmasked text frame.
func (c *wsConn) writeMessage(payload []byte) error {
	return c.writeFrame(wsOpcodeText, payload)
}

func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var header []byte
	length := len(payload)
	switch {
	case length < wsLen16:
		header = []byte{wsFinBit | opcode, byte(length)}
	case length <= math.MaxUint16:
		header = make([]byte, 4)
		header[0] = wsFinBit | opcode
		header[1] = wsLen16
		binary.BigEndian.PutUint16(header[2:], uint16(length))
	default:
		header = make([]byte, 10)
		header[0] = wsFinBit | opcode
		header[1] = wsLen64
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func (c *wsConn) close() error {
	_ = c.writeFrame(wsOpcodeClose, nil)
	return c.conn.Close()
}

// wsClient is one active /v1/sessions/{id}/events connection. Events are
// queued on outbox and delivered by a dedicated writer goroutine so that a
// slow or stuck client cannot block announce delivery to other clients.
type wsClient struct {
	conn    *wsConn
	session *session
	server  *Server

	outbox       chan []byte
	done         chan struct{}
	writable     chan struct{}
	writableOnce sync.Once
	closeOnce    sync.Once

	announceFilterMu sync.Mutex
	announceFilter   string // empty = all, else exact 16-byte dest hash hex
}

const wsClientOutboxSize = 32

func newWSClient(server *Server, sess *session, conn *wsConn) *wsClient {
	return &wsClient{
		conn:     conn,
		session:  sess,
		server:   server,
		outbox:   make(chan []byte, wsClientOutboxSize),
		done:     make(chan struct{}),
		writable: make(chan struct{}),
	}
}

// startWriter launches writeLoop before the handshake is flushed so the
// writer exists by the time the peer can observe a ready socket. writeLoop
// blocks on writable until enableWrites, so frames cannot interleave with
// the HTTP 101 response.
func (c *wsClient) startWriter() {
	go c.writeLoop()
}

// enableWrites unblocks writeLoop after the handshake has been flushed.
func (c *wsClient) enableWrites() {
	c.writableOnce.Do(func() { close(c.writable) })
}

// run blocks in the read loop until the connection closes. The caller must
// already have registered the client, started the writer, flushed the
// handshake, and called enableWrites.
func (c *wsClient) run() {
	c.readLoop()
	c.close()
}

func (c *wsClient) writeLoop() {
	select {
	case <-c.writable:
	case <-c.done:
		return
	}
	for {
		select {
		case msg := <-c.outbox:
			if err := c.conn.writeMessage(msg); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *wsClient) readLoop() {
	for {
		msg, err := c.conn.readMessage()
		if err != nil {
			return
		}
		c.handleCommand(msg)
	}
}

func (c *wsClient) handleCommand(raw []byte) {
	var env wsCommandEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Error: "invalid command json"})
		return
	}
	switch env.Type {
	case "subscribe_announces":
		c.handleSubscribeAnnounces(raw)
	case "link.open":
		c.handleLinkOpen(raw)
	case "link.send":
		c.handleLinkSend(raw)
	case "link.close":
		c.handleLinkClose(raw)
	case "link.request":
		c.handleLinkRequest(raw)
	case "link.send_resource":
		c.handleLinkSendResource(raw)
	case "link.identify":
		c.handleLinkIdentify(raw)
	case "request.respond":
		c.handleRequestRespond(raw)
	default:
		c.send(commandErrorEvent{Type: "command.error", Command: env.Type, Error: "unknown command"})
	}
}

func (c *wsClient) handleSubscribeAnnounces(raw []byte) {
	var cmd subscribeAnnouncesCommand
	if err := json.Unmarshal(raw, &cmd); err != nil {
		c.send(commandErrorEvent{Type: "command.error", Command: "subscribe_announces", Error: "invalid command json"})
		return
	}
	if cmd.Filter != "" {
		b, err := hex.DecodeString(cmd.Filter)
		if err != nil || len(b) != 16 {
			c.send(commandErrorEvent{Type: "command.error", Command: "subscribe_announces", Error: "filter must be 16 hex-encoded bytes or empty"})
			return
		}
		cmd.Filter = hex.EncodeToString(b)
	}
	c.announceFilterMu.Lock()
	c.announceFilter = cmd.Filter
	c.announceFilterMu.Unlock()
	c.server.subscribeAnnounces(c)
}

func (c *wsClient) matchesAnnounceFilter(destHashHex string) bool {
	c.announceFilterMu.Lock()
	filter := c.announceFilter
	c.announceFilterMu.Unlock()
	if filter == "" {
		return true
	}
	return filter == destHashHex
}

// send enqueues v for delivery, dropping it if the client is not draining
// its outbox fast enough.
func (c *wsClient) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.outbox <- data:
	case <-c.done:
	default:
		debug.Log(debug.DebugError, "controlapi: dropping event, client outbox full")
	}
}

func (c *wsClient) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.close()
		c.server.unsubscribeAnnounces(c)
		c.session.removeClient(c)
	})
}
