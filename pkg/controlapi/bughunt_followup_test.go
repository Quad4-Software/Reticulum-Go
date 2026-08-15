// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/identity"
)

// TestBughuntHTTPBodyTooLargeRejectsOversizedJSON ensures HTTP bodies cannot
// exceed the same 1 MiB cap as inbound WebSocket frames.
func TestBughuntHTTPBodyTooLargeRejectsOversizedJSON(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	authKey := hex.EncodeToString(key)

	huge := `{"app_name":"` + strings.Repeat("a", maxHTTPBodyBytes+64) + `"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/v1/sessions", strings.NewReader(huge))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+authKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s want 413", resp.StatusCode, body)
	}
}

// TestBughuntRequestRespondTimeoutOwnership ensures a respond that races the
// timeout cannot report success to the WS client while the link handler got nil.
func TestBughuntRequestRespondTimeoutOwnership(t *testing.T) {
	prev := requestResponseTimeout
	requestResponseTimeout = 40 * time.Millisecond
	t.Cleanup(func() { requestResponseTimeout = prev })

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession("timeout-race", id)

	const rounds = 80
	for round := range rounds {
		reqID := fmt.Sprintf("req-%d", round)
		ch := sess.awaitResponse(reqID)

		var handlerResult any
		var handlerDone sync.WaitGroup
		handlerDone.Go(func() {
			select {
			case resp := <-ch:
				handlerResult = resp
			case <-time.After(requestResponseTimeout):
				if !sess.forgetResponse(reqID) {
					handlerResult = <-ch
					return
				}
				handlerResult = nil
			}
		})

		time.Sleep(time.Duration(round%7) * time.Millisecond)
		delivered := sess.deliverResponse(reqID, []byte("pong"))
		handlerDone.Wait()

		if delivered && handlerResult == nil {
			t.Fatalf("round %d: deliver reported success but handler got nil", round)
		}
		if !delivered && handlerResult != nil {
			if b, ok := handlerResult.([]byte); !ok || !bytes.Equal(b, []byte("pong")) {
				t.Fatalf("round %d: unexpected handler result %#v after failed deliver", round, handlerResult)
			}
		}
		if delivered {
			b, ok := handlerResult.([]byte)
			if !ok || !bytes.Equal(b, []byte("pong")) {
				t.Fatalf("round %d: delivered but handler got %#v", round, handlerResult)
			}
		}
	}
}
