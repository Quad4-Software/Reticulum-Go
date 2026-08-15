// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/identity"
)

// TestBughuntUnmaskedClientFrameRejected enforces RFC 6455 section 5.1.
func TestBughuntUnmaskedClientFrameRejected(t *testing.T) {
	raw := []byte{0x81, 0x01, 'x'}
	conn := &wsConn{conn: discardConn{}, reader: bufio.NewReader(bytes.NewReader(raw))}
	if _, err := conn.readMessage(); err == nil {
		t.Fatal("unmasked client frame accepted")
	}
}

func TestBughuntEnableWritesIdempotent(t *testing.T) {
	srv, _ := newTestServer(t)
	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	c := newWSClient(srv, newSession("once", ident), &wsConn{conn: discardConn{}, reader: bufio.NewReader(bytes.NewReader(nil))})
	c.enableWrites()
	c.enableWrites()
}

// TestBughuntSessionDeleteRejectsLateAddClient closes the race where a
// WebSocket dial could attach after DELETE had already torn the session down.
func TestBughuntSessionDeleteRejectsLateAddClient(t *testing.T) {
	srv, _ := newTestServer(t)
	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession("closed-late", ident)
	sess.close()
	c := newWSClient(srv, sess, &wsConn{conn: discardConn{}, reader: bufio.NewReader(bytes.NewReader(nil))})
	if sess.addClient(c) {
		t.Fatal("addClient succeeded on closed session")
	}
}

// TestBughuntDeleteVsDialStress races DELETE with /events upgrades and
// asserts deleted sessions never remain in the server map.
func TestBughuntDeleteVsDialStress(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	const rounds = 40
	for round := range rounds {
		_, sessResp := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
		sessionID, _ := sessResp["session_id"].(string)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/v1/sessions/%s", ts.URL, sessionID), nil)
			req.Header.Set("Authorization", "Bearer "+authKey)
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		go func() {
			defer wg.Done()
			conn := softDialControlAPIWS(ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
			if conn != nil {
				_ = conn.Close()
			}
		}()
		wg.Wait()

		srv.mu.RLock()
		_, stillListed := srv.sessions[sessionID]
		srv.mu.RUnlock()
		if stillListed {
			t.Fatalf("round %d: deleted session still in map", round)
		}
	}
}

func softDialControlAPIWS(httpURL, path, authKeyHex string) net.Conn {
	addr := strings.TrimPrefix(httpURL, "http://")
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return nil
	}
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	wsKey := base64.StdEncoding.EncodeToString(keyBytes)
	req := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nAuthorization: Bearer %s\r\n\r\n",
		path, addr, wsKey, authKeyHex,
	)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return nil
	}
	buf := make([]byte, 256)
	_, _ = conn.Read(buf)
	return conn
}
