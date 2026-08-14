// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

// newTestServer builds a Server bound to a real, otherwise-idle Transport.
// No network interfaces are registered, so nothing on these tests ever
// touches the network.
func newTestServer(t testing.TB) (*Server, []byte) {
	t.Helper()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	cfg := common.DefaultConfig()
	cfg.RPCKey = key

	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	srv, err := New(tr, nil, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv, key
}

func doJSON(t testing.TB, method, url, authKeyHex string, body any) (*http.Response, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+authKeyHex)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var decoded map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("decode body %q: %v", raw, err)
		}
	}
	return resp, decoded
}

func TestSessionDestinationAnnounceLifecycle(t *testing.T) {
	srv, key := newTestServer(t)
	iface := newPipeInterface("announce-test")
	if err := srv.transport.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	resp, session := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	sessionID, _ := session["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("create session: missing session_id in %v", session)
	}

	destPath := fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID)

	resp, errBody := doJSON(t, http.MethodPost, destPath, authKey, map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("register destination without app_name status = %d, want %d (body %v)", resp.StatusCode, http.StatusBadRequest, errBody)
	}

	resp, dest := doJSON(t, http.MethodPost, destPath, authKey, map[string]any{
		"app_name": "controlapi_test",
		"aspects":  []string{"unit"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register destination status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	destHash, _ := dest["destination_hash"].(string)
	if len(destHash) != 32 { // 16 bytes, hex-encoded
		t.Fatalf("destination_hash = %q, want 32 hex chars", destHash)
	}

	announcePath := fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/announce", ts.URL, sessionID, destHash)

	resp, _ = doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/deadbeefdeadbeefdeadbeefdeadbeef/announce", ts.URL, sessionID), authKey, map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("announce unknown destination status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	resp, _ = doJSON(t, http.MethodPost, announcePath, authKey, map[string]any{
		"app_data": base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("announce status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	resp, _ = doJSON(t, http.MethodDelete, ts.URL+"/v1/sessions/"+sessionID, authKey, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete session status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	resp, _ = doJSON(t, http.MethodDelete, ts.URL+"/v1/sessions/"+sessionID, authKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete already-deleted session status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestPathRequestValidation(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, session := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := session["session_id"].(string)

	pathURL := fmt.Sprintf("%s/v1/sessions/%s/path/request", ts.URL, sessionID)

	resp, _ := doJSON(t, http.MethodPost, pathURL, authKey, map[string]any{"destination_hash": "not-hex"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad hash status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp, _ = doJSON(t, http.MethodPost, pathURL, authKey, map[string]any{"destination_hash": "aabb"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("short hash status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	valid16 := hex.EncodeToString(make([]byte, 16))
	resp, body := doJSON(t, http.MethodPost, pathURL, authKey, map[string]any{"destination_hash": valid16})
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("valid hash status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	if _, ok := body["wait_s"]; !ok {
		t.Errorf("accepted path request missing wait_s: %v", body)
	}

	resp, body = doJSON(t, http.MethodPost, pathURL, authKey, map[string]any{"destination_hash": valid16})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("throttled path request status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if body["error"] == "" {
		t.Errorf("throttled path request missing error: %v", body)
	}

	unknownSessionURL := fmt.Sprintf("%s/v1/sessions/does-not-exist/path/request", ts.URL)
	resp, _ = doJSON(t, http.MethodPost, unknownSessionURL, authKey, map[string]any{"destination_hash": valid16})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown session status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// testWSClient is a minimal RFC 6455 client used only to exercise the
// server's real websocket handshake and framing from outside the package.
type testWSClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dialControlAPIWS(t testing.TB, httpURL, path, authKeyHex string) *testWSClient {
	t.Helper()

	addr := strings.TrimPrefix(httpURL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatalf("generate ws key: %v", err)
	}
	wsKey := base64.StdEncoding.EncodeToString(keyBytes)

	request := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nAuthorization: Bearer %s\r\n\r\n",
		path, addr, wsKey, authKeyHex,
	)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line: %v", err)
	}
	if !strings.Contains(statusLine, "101") {
		t.Fatalf("handshake status line = %q, want 101", statusLine)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	return &testWSClient{conn: conn, reader: reader}
}

func (c *testWSClient) sendText(t testing.TB, payload []byte) {
	t.Helper()
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	var header []byte
	switch {
	case len(payload) < 126:
		header = []byte{0x81, byte(0x80 | len(payload))}
	case len(payload) <= 65535:
		header = make([]byte, 4)
		header[0] = 0x81
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:], uint16(len(payload)))
	default:
		t.Fatalf("test helper payload too large: %d", len(payload))
	}
	frame := append(header, mask...)
	frame = append(frame, masked...)
	if _, err := c.conn.Write(frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

func (c *testWSClient) recvText(t testing.TB, timeout time.Duration) []byte {
	t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		t.Fatalf("read frame header: %v", err)
	}
	length := int(header[1] & 0x7F)
	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, ext); err != nil {
			t.Fatalf("read extended length: %v", err)
		}
		length = int(binary.BigEndian.Uint16(ext))
	case 127:
		t.Fatalf("test helper does not support 64-bit frame lengths")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		t.Fatalf("read frame payload: %v", err)
	}
	return payload
}

func TestWebSocketAnnounceSubscription(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, session := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := session["session_id"].(string)

	ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
	defer ws.conn.Close()

	ws.sendText(t, []byte(`{"type":"subscribe_announces"}`))

	// Give the read loop a moment to process the subscribe command before
	// the announce fires, since delivery is asynchronous.
	time.Sleep(50 * time.Millisecond)

	ident, err := identity.NewIdentity()
	if err != nil {
		t.Fatalf("new identity: %v", err)
	}
	destHash := bytes.Repeat([]byte{0xAB}, 16)
	bridge := &announceBridge{server: srv}
	if err := bridge.ReceivedAnnounce(destHash, ident, []byte("payload"), 3); err != nil {
		t.Fatalf("ReceivedAnnounce: %v", err)
	}

	raw := ws.recvText(t, 2*time.Second)
	var evt announceEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatalf("decode event %q: %v", raw, err)
	}
	if evt.Type != "announce" {
		t.Errorf("event type = %q, want %q", evt.Type, "announce")
	}
	if evt.DestinationHash != hex.EncodeToString(destHash) {
		t.Errorf("destination_hash = %q, want %q", evt.DestinationHash, hex.EncodeToString(destHash))
	}
	if evt.IdentityHash != ident.GetHexHash() {
		t.Errorf("identity_hash = %q, want %q", evt.IdentityHash, ident.GetHexHash())
	}
	if evt.Hops != 3 {
		t.Errorf("hops = %d, want 3", evt.Hops)
	}
}
