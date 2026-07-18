// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"quad4/reticulum-go/pkg/identity"
)

func FuzzWSCommandDecode(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"type":"link.open"}`))
	f.Add([]byte(`{"type":"link.send","link_id":"00","data":"YQ=="}`))
	f.Add([]byte(`{"type":"link.request","link_id":"00","path":"/x"}`))
	f.Add([]byte(`{"type":"link.send_resource","link_id":"00","data":"YQ=="}`))
	f.Add([]byte(`{"type":"link.identify","link_id":"00"}`))
	f.Add([]byte(`{"type":"request.respond","request_id":"00"}`))
	f.Add([]byte(`{"type":"subscribe_announces","filter":"11111111111111111111111111111111"}`))
	f.Add([]byte(`{not-json`))
	f.Add([]byte{0xff, 0xfe, 0x00})

	srv, _ := newTestServer(f)
	ident, err := identity.NewIdentity()
	if err != nil {
		f.Fatal(err)
	}
	sess := newSession("fuzz", ident)
	c := &wsClient{
		session:  sess,
		server:   srv,
		outbox:   make(chan []byte, 256),
		done:     make(chan struct{}),
		writable: make(chan struct{}),
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 8192 {
			raw = raw[:8192]
		}
		// Drain outbox so a full buffer cannot block send forever.
		for {
			select {
			case <-c.outbox:
			default:
				c.handleCommand(raw)
				return
			}
		}
	})
}

func FuzzHTTPRegisterBodies(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"app_name":"x"}`))
	f.Add([]byte(`{"app_name":"x","aspects":["a"],"accepts_links":true}`))
	f.Add([]byte(`{"path":"/ping"}`))
	f.Add([]byte(`{"path":"/ping","allow":"list","allowed_identities":["00112233445566778899aabbccddeeff"]}`))
	f.Add([]byte(`{"destination_hash":"00112233445566778899aabbccddeeff"}`))
	f.Add([]byte(`{"app_data":"YQ=="}`))
	f.Add([]byte(`{`))
	f.Add([]byte{0x00, 0xff})

	srv, key := newTestServer(f)
	ts := httptest.NewServer(srv.httpServer.Handler)
	f.Cleanup(ts.Close)
	authKey := hex.EncodeToString(key)

	_, sess := doJSON(f, http.MethodPost, ts.URL+"/v1/sessions", authKey, map[string]any{})
	sessionID, _ := sess["session_id"].(string)
	_, dest := doJSON(f, http.MethodPost, fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID), authKey, map[string]any{"app_name": "fuzz"})
	destHash, _ := dest["destination_hash"].(string)

	paths := []string{
		fmt.Sprintf("%s/v1/sessions", ts.URL),
		fmt.Sprintf("%s/v1/sessions/%s/destinations", ts.URL, sessionID),
		fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/announce", ts.URL, sessionID, destHash),
		fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", ts.URL, sessionID, destHash),
		fmt.Sprintf("%s/v1/sessions/%s/path/request", ts.URL, sessionID),
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 8192 {
			body = body[:8192]
		}
		for _, url := range paths {
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+authKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 600 {
				t.Fatalf("unexpected status %d for %s", resp.StatusCode, url)
			}
		}
	})
}
