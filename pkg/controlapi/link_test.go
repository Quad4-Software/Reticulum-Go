// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

// TestRequestResponseBridgeDirect exercises wireRequestHandler's blocking
// bridge in isolation, invoking the registered handler directly instead of
// through a real link, to make failures easy to attribute.
func TestRequestResponseBridgeDirect(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, sessResp := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := sessResp["session_id"].(string)

	_, destResp := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID), authKey, map[string]any{"app_name": "bridge_test"})
	destHash, _ := destResp["destination_hash"].(string)

	resp, _ := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", ts.URL, sessionID, destHash), authKey, map[string]any{"path": "/ping"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register request handler status = %d", resp.StatusCode)
	}

	ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
	defer ws.conn.Close()

	sess, ok := srv.session(sessionID)
	if !ok {
		t.Fatal("session not found")
	}
	dest, ok := sess.destination(destHash)
	if !ok {
		t.Fatal("destination not found")
	}
	pathHash := identity.TruncatedHash([]byte("/ping"))
	handler := dest.GetRequestHandler(pathHash)
	if handler == nil {
		t.Fatal("no handler registered for /ping")
	}

	requestID := []byte{1, 2, 3, 4}
	linkID := []byte{5, 6, 7, 8}
	resultCh := make(chan any, 1)
	go func() {
		resultCh <- handler(pathHash, []byte("hello"), requestID, linkID, nil, time.Now())
	}()

	raw := ws.recvText(t, 2*time.Second)
	var incoming requestIncomingEvent
	if err := json.Unmarshal(raw, &incoming); err != nil {
		t.Fatalf("decode event %q: %v", raw, err)
	}
	if incoming.Type != "request.incoming" || incoming.Path != "/ping" {
		t.Fatalf("request.incoming = %+v", incoming)
	}
	if incoming.RequestID != hex.EncodeToString(requestID) {
		t.Fatalf("request_id = %q, want %q", incoming.RequestID, hex.EncodeToString(requestID))
	}

	select {
	case <-resultCh:
		t.Fatal("handler returned before request.respond was sent")
	case <-time.After(200 * time.Millisecond):
	}

	ws.sendText(t, []byte(fmt.Sprintf(`{"type":"request.respond","request_id":%q,"data":%q}`,
		incoming.RequestID, base64.StdEncoding.EncodeToString([]byte("pong")))))

	select {
	case result := <-resultCh:
		b, ok := result.([]byte)
		if !ok || string(b) != "pong" {
			t.Fatalf("handler result = %#v, want []byte(\"pong\")", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never returned after request.respond")
	}
}

func TestParseAllowMode(t *testing.T) {
	if allow, list, err := parseAllowMode("", nil); err != nil || allow != 0x01 || list != nil {
		t.Errorf("default allow = (%v, %v, %v), want (AllowAll, nil, nil)", allow, list, err)
	}
	if allow, _, err := parseAllowMode("all", nil); err != nil || allow != 0x01 {
		t.Errorf("allow=all -> (%v, %v)", allow, err)
	}
	if allow, _, err := parseAllowMode("none", nil); err != nil || allow != 0x00 {
		t.Errorf("allow=none -> (%v, %v)", allow, err)
	}
	if _, _, err := parseAllowMode("list", nil); err == nil {
		t.Error("allow=list with no identities should error")
	}
	hashHex := hex.EncodeToString(bytes.Repeat([]byte{0xAA}, 16))
	allow, list, err := parseAllowMode("list", []string{hashHex})
	if err != nil || allow != 0x02 || len(list) != 1 {
		t.Errorf("allow=list -> (%v, %v, %v)", allow, list, err)
	}
	if _, _, err := parseAllowMode("list", []string{"not-hex"}); err == nil {
		t.Error("allow=list with invalid hex should error")
	}
	if _, _, err := parseAllowMode("bogus", nil); err == nil {
		t.Error("unknown allow mode should error")
	}
}

func TestLinkOpenValidation(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, session := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := session["session_id"].(string)

	ws := dialControlAPIWS(t, ts.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionID), authKey)
	defer ws.conn.Close()

	ws.sendText(t, []byte(`{"type":"link.open","destination_hash":"not-hex"}`))
	raw := ws.recvText(t, 2*time.Second)
	var evt linkFailedEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatalf("decode event %q: %v", raw, err)
	}
	if evt.Type != "link.failed" {
		t.Errorf("event type = %q, want link.failed", evt.Type)
	}

	unknown := hex.EncodeToString(bytes.Repeat([]byte{0xCD}, 16))
	ws.sendText(t, []byte(fmt.Sprintf(`{"type":"link.open","destination_hash":%q}`, unknown)))
	raw = ws.recvText(t, 2*time.Second)
	if err := json.Unmarshal(raw, &evt); err != nil {
		t.Fatalf("decode event %q: %v", raw, err)
	}
	if evt.Type != "link.failed" || evt.DestinationHash != unknown {
		t.Errorf("event = %+v, want link.failed for %q", evt, unknown)
	}
}

func TestRegisterRequestHandlerValidation(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	_, session := doJSON(t, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := session["session_id"].(string)

	_, dest := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID), authKey, map[string]any{
		"app_name": "controlapi_test",
	})
	destHash, _ := dest["destination_hash"].(string)

	reqURL := fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", ts.URL, sessionID, destHash)

	resp, _ := doJSON(t, http.MethodPost, reqURL, authKey, map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing path status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp, _ = doJSON(t, http.MethodPost, reqURL, authKey, map[string]any{"path": "/ping", "allow": "bogus"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad allow mode status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp, _ = doJSON(t, http.MethodPost, reqURL, authKey, map[string]any{"path": "/ping"})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("valid registration status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	unknownDestURL := fmt.Sprintf("%s/v1/sessions/%s/destinations/deadbeefdeadbeefdeadbeefdeadbeef/requests", ts.URL, sessionID)
	resp, _ = doJSON(t, http.MethodPost, unknownDestURL, authKey, map[string]any{"path": "/ping"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown destination status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// pipeInterface simulates a direct, lossless connection between two nodes'
// transports so link establishment can be exercised without real sockets.
type pipeInterface struct {
	common.BaseInterface
	peer *pipeInterface
	tr   *transport.Transport
}

func newPipeInterface(name string) *pipeInterface {
	return &pipeInterface{
		BaseInterface: common.BaseInterface{
			Name:    name,
			Type:    common.IFTypeUDP,
			Enabled: true,
			Online:  true,
		},
	}
}

func (p *pipeInterface) Send(data []byte, address string) error {
	if p.peer == nil || p.peer.tr == nil {
		return nil
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	p.peer.tr.HandlePacket(dataCopy, p.peer)
	return nil
}

func (p *pipeInterface) IsEnabled() bool { return true }
func (p *pipeInterface) IsOnline() bool  { return true }
func (p *pipeInterface) GetName() string { return p.Name }
func (p *pipeInterface) Start() error    { return nil }
func (p *pipeInterface) Stop() error     { return nil }
func (p *pipeInterface) Detach()         {}

// newLinkTestServer is like newTestServer but returns the underlying
// Transport too, so tests can wire a pipeInterface between two nodes.
func newLinkTestServer(t *testing.T) (*Server, *transport.Transport, []byte) {
	t.Helper()

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	cfg := common.DefaultConfig()
	cfg.RPCKey = key

	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	srv, err := New(tr, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv, tr, key
}

// TestLinkAndRequestLifecycle drives two independent control API servers,
// connected by an in-process pipe interface, through the full phase-2
// surface: an inbound-accepting destination on node A, an outbound
// link.open from node B, bidirectional link.send/link.data, a
// request.incoming/request.respond round trip, and link.close/link.closed
// on both ends.
func TestLinkAndRequestLifecycle(t *testing.T) {
	srvA, trA, keyA := newLinkTestServer(t)
	srvB, trB, keyB := newLinkTestServer(t)

	pipeA := newPipeInterface("pipeA")
	pipeB := newPipeInterface("pipeB")
	pipeA.peer = pipeB
	pipeB.peer = pipeA
	pipeA.tr = trA
	pipeB.tr = trB
	if err := trA.RegisterInterface("pipeA", pipeA); err != nil {
		t.Fatalf("register pipeA: %v", err)
	}
	if err := trB.RegisterInterface("pipeB", pipeB); err != nil {
		t.Fatalf("register pipeB: %v", err)
	}
	_ = trA.InitializePathRequestHandler()

	tsA := httptest.NewServer(srvA.httpServer.Handler)
	defer tsA.Close()
	tsB := httptest.NewServer(srvB.httpServer.Handler)
	defer tsB.Close()
	authA := hex.EncodeToString(keyA)
	authB := hex.EncodeToString(keyB)

	_, sessA := doJSON(t, http.MethodPost, tsA.URL+"/v1/sessions", authA, map[string]any{})
	sessionIDA, _ := sessA["session_id"].(string)
	_, sessB := doJSON(t, http.MethodPost, tsB.URL+"/v1/sessions", authB, map[string]any{})
	sessionIDB, _ := sessB["session_id"].(string)

	resp, destA := doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", tsA.URL, sessionIDA), authA, map[string]any{
		"app_name":      "controlapi_test",
		"aspects":       []string{"link"},
		"accepts_links": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register destination A status = %d", resp.StatusCode)
	}
	destHashA, _ := destA["destination_hash"].(string)

	const pingResponse = "pong"
	resp, _ = doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", tsA.URL, sessionIDA, destHashA), authA, map[string]any{
		"path": "/ping",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register request handler status = %d", resp.StatusCode)
	}

	wsA := dialControlAPIWS(t, tsA.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionIDA), authA)
	defer wsA.conn.Close()

	resp, _ = doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/announce", tsA.URL, sessionIDA, destHashA), authA, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("announce status = %d", resp.StatusCode)
	}

	destHashABytes, err := hex.DecodeString(destHashA)
	if err != nil {
		t.Fatalf("decode dest hash: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !trB.HasPath(destHashABytes) {
		if time.Now().After(deadline) {
			t.Fatal("node B never learned a path to node A's destination")
		}
		time.Sleep(20 * time.Millisecond)
	}

	wsB := dialControlAPIWS(t, tsB.URL, fmt.Sprintf("/v1/sessions/%s/events", sessionIDB), authB)
	defer wsB.conn.Close()

	wsB.sendText(t, []byte(fmt.Sprintf(`{"type":"link.open","destination_hash":%q}`, destHashA)))

	establishedB := decodeEvent[linkEstablishedEvent](t, wsB, 5*time.Second)
	if establishedB.Type != "link.established" || establishedB.LinkID == "" {
		t.Fatalf("node B link.established = %+v", establishedB)
	}
	establishedA := decodeEvent[linkEstablishedEvent](t, wsA, 5*time.Second)
	if establishedA.Type != "link.established" || establishedA.LinkID == "" {
		t.Fatalf("node A link.established = %+v", establishedA)
	}

	internalSessB, ok := srvB.session(sessionIDB)
	if !ok {
		t.Fatal("session B not found")
	}
	lsB, ok := internalSessB.getLink(establishedB.LinkID)
	if !ok {
		t.Fatalf("link %s not tracked in session B", establishedB.LinkID)
	}

	receipt, err := lsB.link.Request("/ping", []byte("hello"), 5*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	incoming := decodeEvent[requestIncomingEvent](t, wsA, 5*time.Second)
	if incoming.Type != "request.incoming" || incoming.Path != "/ping" {
		t.Fatalf("request.incoming = %+v", incoming)
	}
	incomingData, err := base64.StdEncoding.DecodeString(incoming.Data)
	if err != nil || string(incomingData) != "hello" {
		t.Fatalf("request.incoming data = %q (err %v), want %q", incoming.Data, err, "hello")
	}

	wsA.sendText(t, []byte(fmt.Sprintf(`{"type":"request.respond","request_id":%q,"data":%q}`,
		incoming.RequestID, base64.StdEncoding.EncodeToString([]byte(pingResponse)))))

	deadline = time.Now().Add(5 * time.Second)
	for !receipt.Concluded() {
		if time.Now().After(deadline) {
			t.Fatal("request never concluded")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := string(receipt.GetResponse()); got != pingResponse {
		t.Errorf("response = %q, want %q", got, pingResponse)
	}

	payload := []byte("link-data-payload")
	wsB.sendText(t, []byte(fmt.Sprintf(`{"type":"link.send","link_id":%q,"data":%q}`,
		establishedB.LinkID, base64.StdEncoding.EncodeToString(payload))))

	dataEvt := decodeEvent[linkDataEvent](t, wsA, 5*time.Second)
	dataBytes, err := base64.StdEncoding.DecodeString(dataEvt.Data)
	if err != nil || string(dataBytes) != string(payload) {
		t.Fatalf("link.data = %q (err %v), want %q", dataEvt.Data, err, payload)
	}

	wsB.sendText(t, []byte(fmt.Sprintf(`{"type":"link.close","link_id":%q}`, establishedB.LinkID)))

	closedB := decodeEvent[linkClosedEvent](t, wsB, 5*time.Second)
	if closedB.LinkID != establishedB.LinkID {
		t.Errorf("node B link.closed link_id = %q, want %q", closedB.LinkID, establishedB.LinkID)
	}
	closedA := decodeEvent[linkClosedEvent](t, wsA, 5*time.Second)
	if closedA.LinkID != establishedA.LinkID {
		t.Errorf("node A link.closed link_id = %q, want %q", closedA.LinkID, establishedA.LinkID)
	}
}

func decodeEvent[T any](t *testing.T, ws *testWSClient, timeout time.Duration) T {
	t.Helper()
	raw := ws.recvText(t, timeout)
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode event %q: %v", raw, err)
	}
	return v
}
