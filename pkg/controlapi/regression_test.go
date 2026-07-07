// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package controlapi

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRegression_LinkLifecycleCompletesWithinDeadline guards against mutex
// deadlocks in the link layer that previously hung link establishment and
// request handling for 10+ minutes. Run with a bounded timeout, e.g.
// go test -timeout 30s ./pkg/controlapi -run Regression_LinkLifecycle
func TestRegression_LinkLifecycleCompletesWithinDeadline(t *testing.T) {
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
		"app_name":      "controlapi_regression",
		"aspects":       []string{"link"},
		"accepts_links": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register destination status = %d", resp.StatusCode)
	}
	destHashA, _ := destA["destination_hash"].(string)

	const pingResponse = "pong"
	resp, _ = doJSON(t, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", tsA.URL, sessionIDA, destHashA), authA, map[string]any{
		"path": "/ping",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register handler status = %d", resp.StatusCode)
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
	pathDeadline := time.Now().Add(5 * time.Second)
	for !trB.HasPath(destHashABytes) {
		if time.Now().After(pathDeadline) {
			t.Fatal("node B never learned path to A")
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

	wsA.sendText(t, []byte(fmt.Sprintf(`{"type":"request.respond","request_id":%q,"data":%q}`,
		incoming.RequestID, base64.StdEncoding.EncodeToString([]byte(pingResponse)))))

	respDeadline := time.Now().Add(5 * time.Second)
	for !receipt.Concluded() {
		if time.Now().After(respDeadline) {
			t.Fatal("request never concluded (possible Request/encrypt deadlock)")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := string(receipt.GetResponse()); got != pingResponse {
		t.Fatalf("response = %q, want %q", got, pingResponse)
	}

	wsB.sendText(t, []byte(fmt.Sprintf(`{"type":"link.close","link_id":%q}`, establishedB.LinkID)))
	closedB := decodeEvent[linkClosedEvent](t, wsB, 5*time.Second)
	if closedB.LinkID != establishedB.LinkID {
		t.Fatalf("node B link.closed link_id = %q, want %q", closedB.LinkID, establishedB.LinkID)
	}
}

// TestRegression_AuthRejectsWrongLengthToken guards against accepting tokens
// whose decoded length differs from the configured rpc_key (timing oracle via
// subtle.ConstantTimeCompare length mismatch).
func TestRegression_AuthRejectsWrongLengthToken(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+hex.EncodeToString(key[:len(key)-1]))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}
