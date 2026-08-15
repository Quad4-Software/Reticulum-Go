// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// waitEventType reads WS events until one matches typ or timeout.
func waitEventType(t testing.TB, ws *testWSClient, typ string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remain := time.Until(deadline)
		if remain <= 0 {
			break
		}
		raw := ws.recvText(t, remain)
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode event %q: %v", raw, err)
		}
		if got, _ := m["type"].(string); got == typ {
			return m
		}
	}
	t.Fatalf("timed out waiting for event type %q", typ)
	return nil
}

func setupLinkedSessions(t *testing.T) (
	srvA, srvB *Server,
	tsA, tsB *httptest.Server,
	authA, authB, sessionIDA, sessionIDB, destHashA string,
	wsA, wsB *testWSClient,
) {
	t.Helper()
	srvA, trA, keyA := newLinkTestServer(t)
	srvB, trBReal, keyB := newLinkTestServer(t)

	pipeA := newPipeInterface("pipeA")
	pipeB := newPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trBReal
	if err := trA.RegisterInterface("pipeA", pipeA); err != nil {
		t.Fatalf("register pipeA: %v", err)
	}
	if err := trBReal.RegisterInterface("pipeB", pipeB); err != nil {
		t.Fatalf("register pipeB: %v", err)
	}
	_ = trA.InitializePathRequestHandler()

	tsA = httptest.NewServer(srvA.httpServer.Handler)
	t.Cleanup(tsA.Close)
	tsB = httptest.NewServer(srvB.httpServer.Handler)
	t.Cleanup(tsB.Close)
	authA = hex.EncodeToString(keyA)
	authB = hex.EncodeToString(keyB)

	_, sessA := doJSON(t, http.MethodPost, tsA.URL+"/v1/sessions", authA, map[string]any{})
	sessionIDA, _ = sessA["session_id"].(string)
	_, sessB := doJSON(t, http.MethodPost, tsB.URL+"/v1/sessions", authB, map[string]any{})
	sessionIDB, _ = sessB["session_id"].(string)

	resp, destA := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", tsA.URL, sessionIDA), authA, map[string]any{
		"app_name":      "controlapi_parity",
		"aspects":       []string{"link"},
		"accepts_links": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register destination A status = %d", resp.StatusCode)
	}
	destHashA, _ = destA["destination_hash"].(string)

	wsA = dialControlAPIWS(t, tsA.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionIDA), authA)
	t.Cleanup(func() { _ = wsA.conn.Close() })

	resp, _ = doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/announce", tsA.URL, sessionIDA, destHashA), authA, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("announce status = %d", resp.StatusCode)
	}

	destHashABytes, err := hex.DecodeString(destHashA)
	if err != nil {
		t.Fatalf("decode dest hash: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !trBReal.HasPath(destHashABytes) {
		if time.Now().After(deadline) {
			t.Fatal("node B never learned a path to node A's destination")
		}
		time.Sleep(20 * time.Millisecond)
	}

	wsB = dialControlAPIWS(t, tsB.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionIDB), authB)
	t.Cleanup(func() { _ = wsB.conn.Close() })

	return srvA, srvB, tsA, tsB, authA, authB, sessionIDA, sessionIDB, destHashA, wsA, wsB
}

func establishLinkAB(t *testing.T, wsA, wsB *testWSClient, destHashA string) (linkIDA, linkIDB string) {
	t.Helper()
	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.open","destination_hash":%q}`, destHashA))
	establishedB := decodeEvent[linkEstablishedEvent](t, wsB, 5*time.Second)
	establishedA := decodeEvent[linkEstablishedEvent](t, wsA, 5*time.Second)
	if establishedB.LinkID == "" || establishedA.LinkID == "" {
		t.Fatalf("links not established A=%+v B=%+v", establishedA, establishedB)
	}
	return establishedA.LinkID, establishedB.LinkID
}

func TestOutboundLinkRequest(t *testing.T) {
	_, _, tsA, _, authA, _, sessionIDA, _, destHashA, wsA, wsB := setupLinkedSessions(t)

	resp, _ := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", tsA.URL, sessionIDA, destHashA), authA, map[string]any{
		"path": "/echo",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register handler status = %d", resp.StatusCode)
	}

	_, linkIDB := establishLinkAB(t, wsA, wsB, destHashA)

	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.request","link_id":%q,"path":"/echo","data":%q,"timeout_ms":5000}`,
		linkIDB, base64.StdEncoding.EncodeToString([]byte("ping"))))

	incoming := waitEventType(t, wsA, "request.incoming", 5*time.Second)
	wsA.sendText(t, fmt.Appendf(nil, `{"type":"request.respond","request_id":%q,"data":%q}`,
		incoming["request_id"], base64.StdEncoding.EncodeToString([]byte("pong"))))

	out := waitEventType(t, wsB, "request.response", 5*time.Second)
	data, err := base64.StdEncoding.DecodeString(out["data"].(string))
	if err != nil || string(data) != "pong" {
		t.Fatalf("request.response data = %#v", out["data"])
	}
}

func TestOutboundLinkRequestErrors(t *testing.T) {
	_, _, _, _, _, _, _, _, destHashA, wsA, wsB := setupLinkedSessions(t)
	_, linkIDB := establishLinkAB(t, wsA, wsB, destHashA)

	wsB.sendText(t, []byte(`{"type":"link.request","link_id":"deadbeef","path":"/x"}`))
	errEvt := waitEventType(t, wsB, "command.error", 2*time.Second)
	if errEvt["command"] != "link.request" {
		t.Fatalf("command.error = %#v", errEvt)
	}

	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.request","link_id":%q,"path":""}`, linkIDB))
	errEvt = waitEventType(t, wsB, "command.error", 2*time.Second)
	if errEvt["error"] == nil {
		t.Fatal("expected error for empty path")
	}

	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.request","link_id":%q,"path":"/x","data":%q}`, linkIDB, "%%%"))
	errEvt = waitEventType(t, wsB, "command.error", 2*time.Second)
	if errEvt["command"] != "link.request" {
		t.Fatalf("bad base64 command.error = %#v", errEvt)
	}
}

func TestFileRespond(t *testing.T) {
	_, _, tsA, _, authA, _, sessionIDA, _, destHashA, wsA, wsB := setupLinkedSessions(t)

	resp, _ := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", tsA.URL, sessionIDA, destHashA), authA, map[string]any{
		"path": "/file",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register handler status = %d", resp.StatusCode)
	}

	_, linkIDB := establishLinkAB(t, wsA, wsB, destHashA)

	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.request","link_id":%q,"path":"/file","timeout_ms":5000}`, linkIDB))
	incoming := waitEventType(t, wsA, "request.incoming", 5*time.Second)
	wsA.sendText(t, fmt.Appendf(nil, `{"type":"request.respond","request_id":%q,"filename":"hi.txt","data":%q}`,
		incoming["request_id"], base64.StdEncoding.EncodeToString([]byte("content"))))

	out := waitEventType(t, wsB, "request.response", 5*time.Second)
	if out["data"] == nil {
		t.Fatalf("expected response data, got %#v", out)
	}
}

func TestRequestRespondUnknownID(t *testing.T) {
	_, _, _, _, _, _, _, _, destHashA, wsA, wsB := setupLinkedSessions(t)
	establishLinkAB(t, wsA, wsB, destHashA)

	wsA.sendText(t, []byte(`{"type":"request.respond","request_id":"00","data":""}`))
	errEvt := waitEventType(t, wsA, "command.error", 2*time.Second)
	if errEvt["command"] != "request.respond" {
		t.Fatalf("command.error = %#v", errEvt)
	}
}

func TestLinkSendResource(t *testing.T) {
	_, _, _, _, _, _, _, _, destHashA, wsA, wsB := setupLinkedSessions(t)
	_, linkIDB := establishLinkAB(t, wsA, wsB, destHashA)

	payload := base64.StdEncoding.EncodeToString([]byte("resource-bytes"))
	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.send_resource","link_id":%q,"data":%q,"name":"note.txt"}`,
		linkIDB, payload))

	_ = waitEventType(t, wsA, "resource.started", 10*time.Second)
	concluded := waitEventType(t, wsA, "resource.concluded", 15*time.Second)
	if success, _ := concluded["success"].(bool); !success {
		t.Fatalf("resource.concluded = %#v", concluded)
	}
	if name, _ := concluded["name"].(string); name != "note.txt" {
		t.Fatalf("resource name = %q, want note.txt", name)
	}
}

func TestLinkSendResourceUnknownLink(t *testing.T) {
	_, _, _, _, _, _, _, _, destHashA, wsA, wsB := setupLinkedSessions(t)
	establishLinkAB(t, wsA, wsB, destHashA)

	wsB.sendText(t, []byte(`{"type":"link.send_resource","link_id":"00","data":"YQ=="}`))
	errEvt := waitEventType(t, wsB, "command.error", 2*time.Second)
	if errEvt["command"] != "link.send_resource" {
		t.Fatalf("command.error = %#v", errEvt)
	}
}

func TestLinkIdentify(t *testing.T) {
	_, srvB, _, _, _, _, _, sessionIDB, destHashA, wsA, wsB := setupLinkedSessions(t)
	linkIDA, linkIDB := establishLinkAB(t, wsA, wsB, destHashA)

	wsB.sendText(t, fmt.Appendf(nil, `{"type":"link.identify","link_id":%q}`, linkIDB))
	identified := waitEventType(t, wsA, "link.remote_identified", 5*time.Second)
	if identified["link_id"] != linkIDA {
		t.Fatalf("remote_identified link_id = %#v want %s", identified["link_id"], linkIDA)
	}
	sessB, ok := srvB.session(sessionIDB)
	if !ok {
		t.Fatal("session B missing")
	}
	wantHash := sessB.identity.GetHexHash()
	if identified["identity_hash"] != wantHash {
		t.Fatalf("identity_hash = %#v want %s", identified["identity_hash"], wantHash)
	}

	wsB.sendText(t, []byte(`{"type":"link.identify","link_id":"00"}`))
	errEvt := waitEventType(t, wsB, "command.error", 2*time.Second)
	if errEvt["command"] != "link.identify" {
		t.Fatalf("command.error = %#v", errEvt)
	}
}

func TestAnnounceFilter(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, sess := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := sess["session_id"].(string)
	ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
	defer ws.conn.Close()

	match := hex.EncodeToString(bytesRepeat(0x11, 16))
	other := hex.EncodeToString(bytesRepeat(0x22, 16))

	ws.sendText(t, []byte(`{"type":"subscribe_announces","filter":"not-hex"}`))
	errEvt := waitEventType(t, ws, "command.error", 2*time.Second)
	if errEvt["command"] != "subscribe_announces" {
		t.Fatalf("command.error = %#v", errEvt)
	}

	ws.sendText(t, fmt.Appendf(nil, `{"type":"subscribe_announces","filter":%q}`, match))
	time.Sleep(50 * time.Millisecond)

	srv.broadcastAnnounce(announceEvent{Type: "announce", DestinationHash: other, Hops: 1})
	srv.broadcastAnnounce(announceEvent{Type: "announce", DestinationHash: match, Hops: 2})

	evt := waitEventType(t, ws, "announce", 2*time.Second)
	if evt["destination_hash"] != match {
		t.Fatalf("announce = %#v, want only matching hash", evt)
	}
}

func TestDeregisterRequestHandler(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, sess := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := sess["session_id"].(string)
	_, dest := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID), authKey, map[string]any{"app_name": "dereg"})
	destHash, _ := dest["destination_hash"].(string)

	base := fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", ts.URL, sessionID, destHash)
	resp, _ := doJSON(t, http.MethodPost, base, authKey, map[string]any{"path": "/ping"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodDelete, base+"?path=/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+authKey)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d", delResp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodDelete, base+"?path=/ping", nil)
	req.Header.Set("Authorization", "Bearer "+authKey)
	delResp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNotFound {
		t.Fatalf("second DELETE status = %d, want 404", delResp.StatusCode)
	}

	resp, _ = doJSON(t, http.MethodPost, base, authKey, map[string]any{"path": "/ping"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("re-register status = %d", resp.StatusCode)
	}
	_ = srv
}

func TestCommandErrorUnknownAndMalformed(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, sess := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := sess["session_id"].(string)
	ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
	defer ws.conn.Close()

	ws.sendText(t, []byte(`{not-json`))
	errEvt := waitEventType(t, ws, "command.error", 2*time.Second)
	if errEvt["error"] == nil {
		t.Fatal("expected malformed json error")
	}

	ws.sendText(t, []byte(`{"type":"no.such.command"}`))
	errEvt = waitEventType(t, ws, "command.error", 2*time.Second)
	if errEvt["command"] != "no.such.command" {
		t.Fatalf("command.error = %#v", errEvt)
	}

	ws.sendText(t, []byte(`{"type":"link.send","link_id":"00","data":"YQ=="}`))
	errEvt = waitEventType(t, ws, "command.error", 2*time.Second)
	if errEvt["command"] != "link.send" {
		t.Fatalf("link.send error = %#v", errEvt)
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
