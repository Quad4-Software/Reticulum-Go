// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/identity"
)

// recordConn is a non-blocking net.Conn that captures Write bytes for
// handshake-gate assertions without the deadlock risk of net.Pipe.
type recordConn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed chan struct{}
}

func newRecordConn() *recordConn {
	return &recordConn{closed: make(chan struct{})}
}

func (c *recordConn) Read([]byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	case <-time.After(50 * time.Millisecond):
		return 0, fmt.Errorf("recordConn: no inbound data")
	}
}

func (c *recordConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	default:
	}
	return c.buf.Write(b)
}

func (c *recordConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	return nil
}

func (c *recordConn) LocalAddr() net.Addr              { return recordAddr("local") }
func (c *recordConn) RemoteAddr() net.Addr             { return recordAddr("remote") }
func (c *recordConn) SetDeadline(time.Time) error      { return nil }
func (c *recordConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordConn) SetWriteDeadline(time.Time) error { return nil }

func (c *recordConn) snapshot() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

type recordAddr string

func (a recordAddr) Network() string { return "record" }
func (a recordAddr) String() string  { return string(a) }

// TestAcceptance_WriteLoopWaitsUntilEnableWrites locks the contract that
// startWriter may run before the HTTP 101 is flushed, but writeLoop must
// not emit WebSocket frames until enableWrites.
func TestAcceptance_WriteLoopWaitsUntilEnableWrites(t *testing.T) {
	srv, _ := newTestServer(t)
	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	sess := newSession("acceptance-writable", ident)

	rc := newRecordConn()
	c := newWSClient(srv, sess, &wsConn{conn: rc, reader: bufio.NewReader(rc)})
	c.startWriter()
	defer c.close()

	payload := []byte(`{"type":"probe","n":1}`)
	select {
	case c.outbox <- payload:
	default:
		t.Fatal("outbox rejected probe frame")
	}

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := rc.snapshot(); len(got) != 0 {
			t.Fatalf("wrote %q before enableWrites", got)
		}
		time.Sleep(5 * time.Millisecond)
	}

	c.enableWrites()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := rc.snapshot()
		if len(got) > 0 {
			if !bytes.Contains(got, payload) {
				t.Fatalf("wrote %q, want payload %q", got, payload)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no frame written after enableWrites")
}

// TestAcceptance_EventBroadcastDuringDialIsDelivered stresses the
// register-before-flush plus writeLoop-before-flush path: a peer that
// triggers a broadcast as soon as the socket is ready must still receive
// the event.
func TestAcceptance_EventBroadcastDuringDialIsDelivered(t *testing.T) {
	const rounds = 40

	for round := range rounds {
		srv, key := newTestServer(t)
		ts := httptest.NewServer(srv.httpServer.Handler)
		authKey := hex.EncodeToString(key)

		_, sessResp := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
		sessionID, _ := sessResp["session_id"].(string)

		_, destResp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID), authKey, map[string]any{
			"app_name": "acceptance_ws_race",
		})
		destHash, _ := destResp["destination_hash"].(string)

		resp, _ := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", ts.URL, sessionID, destHash), authKey, map[string]any{
			"path": "/ping",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("round %d: register request handler status = %d", round, resp.StatusCode)
		}

		sess, ok := srv.session(sessionID)
		if !ok {
			t.Fatalf("round %d: session not found", round)
		}
		dest, ok := sess.destination(destHash)
		if !ok {
			t.Fatalf("round %d: destination not found", round)
		}
		pathHash := identity.TruncatedHash([]byte("/ping"))
		handler := dest.GetRequestHandler(pathHash)
		if handler == nil {
			t.Fatalf("round %d: no handler for /ping", round)
		}

		requestID := []byte{byte(round), 2, 3, 4}
		linkID := []byte{5, 6, 7, 8}
		resultCh := make(chan any, 1)

		ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)

		go func() {
			resultCh <- handler(pathHash, []byte("hello"), requestID, linkID, nil, time.Now())
		}()

		raw := ws.recvText(t, 5*time.Second)
		var incoming requestIncomingEvent
		if err := json.Unmarshal(raw, &incoming); err != nil {
			_ = ws.conn.Close()
			ts.Close()
			t.Fatalf("round %d: decode event %q: %v", round, raw, err)
		}
		if incoming.Type != "request.incoming" || incoming.Path != "/ping" {
			_ = ws.conn.Close()
			ts.Close()
			t.Fatalf("round %d: request.incoming = %+v", round, incoming)
		}

		ws.sendText(t, fmt.Appendf(nil, `{"type":"request.respond","request_id":%q,"data":%q}`,
			incoming.RequestID, base64.StdEncoding.EncodeToString([]byte("pong"))))

		select {
		case result := <-resultCh:
			b, ok := result.([]byte)
			if !ok || string(b) != "pong" {
				_ = ws.conn.Close()
				ts.Close()
				t.Fatalf("round %d: handler result = %#v, want []byte(\"pong\")", round, result)
			}
		case <-time.After(2 * time.Second):
			_ = ws.conn.Close()
			ts.Close()
			t.Fatalf("round %d: handler never returned after request.respond", round)
		}

		_ = ws.conn.Close()
		ts.Close()
	}
}
