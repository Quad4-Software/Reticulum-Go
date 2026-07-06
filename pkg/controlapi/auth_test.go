// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareRejectsMissingOrWrongToken(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"wrong scheme", "Token " + hex.EncodeToString(key)},
		{"not hex", "Bearer not-hex"},
		{"wrong key", "Bearer " + hex.EncodeToString([]byte("the-wrong-key-entirely!"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestAuthMiddlewareAcceptsValidToken(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+hex.EncodeToString(key))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
