// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package controlapi

import (
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/transport"
)

type mockLifecycle struct {
	resumeN  atomic.Int32
	pauseN   atomic.Int32
	refreshN atomic.Int32
	fail     error
	lastDest [][]byte
}

func (m *mockLifecycle) OnNetworkAvailable() error {
	m.resumeN.Add(1)
	return m.fail
}

func (m *mockLifecycle) OnNetworkLost() error {
	m.pauseN.Add(1)
	return m.fail
}

func (m *mockLifecycle) RefreshPaths(dests ...[]byte) error {
	m.refreshN.Add(1)
	m.lastDest = append([][]byte(nil), dests...)
	return m.fail
}

func freeTCPPort(t testing.TB) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestListenServeStatusPathsClose(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	cfg := common.DefaultConfig()
	cfg.RPCKey = key
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = freeTCPPort(t)

	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	srv, err := New(tr, nil, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if err := srv.Listen(); err != nil {
		t.Fatalf("second Listen should be no-op: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	auth := hex.EncodeToString(key)
	base := "http://" + net.JoinHostPort(cfg.ControlAPIHost, strconv.Itoa(cfg.ControlAPIPort))

	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	var ready bool
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, base+"/v1/health", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+auth)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("server never became ready")
	}

	resp, _ := doJSON(t, http.MethodGet, base+"/v1/status", auth, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/status status = %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, base+"/v1/paths", nil)
	if err != nil {
		t.Fatalf("paths request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+auth)
	pathResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/paths: %v", err)
	}
	_ = pathResp.Body.Close()
	if pathResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/paths status = %d", pathResp.StatusCode)
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

func TestServeBindsWhenListenSkipped(t *testing.T) {
	key := make([]byte, 32)
	cfg := common.DefaultConfig()
	cfg.RPCKey = key
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = freeTCPPort(t)
	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	srv, err := New(tr, nil, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })

	auth := hex.EncodeToString(key)
	base := "http://" + net.JoinHostPort(cfg.ControlAPIHost, strconv.Itoa(cfg.ControlAPIPort))
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, base+"/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+auth)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("Serve did not become ready without prior Listen")
}

func TestListenPortConflict(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer holder.Close()
	port := holder.Addr().(*net.TCPAddr).Port

	key := make([]byte, 32)
	cfg := common.DefaultConfig()
	cfg.RPCKey = key
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = port

	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	srv, err := New(tr, nil, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = srv.Listen()
	if !errors.Is(err, common.ErrPortConflict) {
		t.Fatalf("Listen on busy port: got %v, want ErrPortConflict", err)
	}
}

func TestLifecycleEndpoints(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(0xa0 + i)
	}
	lc := &mockLifecycle{}
	cfg := common.DefaultConfig()
	cfg.RPCKey = key
	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	srv, err := New(tr, lc, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	auth := hex.EncodeToString(key)

	resp, body := doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/resume", auth, nil)
	if resp.StatusCode != http.StatusOK || body["status"] != "resumed" {
		t.Fatalf("resume: status=%d body=%v", resp.StatusCode, body)
	}
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/pause", auth, nil)
	if resp.StatusCode != http.StatusOK || body["status"] != "paused" {
		t.Fatalf("pause: status=%d body=%v", resp.StatusCode, body)
	}

	dest := make([]byte, 16)
	for i := range dest {
		dest[i] = byte(i)
	}
	resp, body = doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/refresh-paths", auth, map[string]any{
		"destinations": []string{hex.EncodeToString(dest)},
	})
	if resp.StatusCode != http.StatusOK || body["status"] != "refreshed" {
		t.Fatalf("refresh: status=%d body=%v", resp.StatusCode, body)
	}
	if lc.resumeN.Load() != 1 || lc.pauseN.Load() != 1 || lc.refreshN.Load() != 1 {
		t.Fatalf("lifecycle counts resume=%d pause=%d refresh=%d", lc.resumeN.Load(), lc.pauseN.Load(), lc.refreshN.Load())
	}
	if len(lc.lastDest) != 1 || hex.EncodeToString(lc.lastDest[0]) != hex.EncodeToString(dest) {
		t.Fatalf("refresh dests = %v", lc.lastDest)
	}

	resp, _ = doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/refresh-paths", auth, map[string]any{
		"destinations": []string{"zz"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad hash status = %d", resp.StatusCode)
	}
}

func TestLifecycleHandlerErrors(t *testing.T) {
	key := make([]byte, 32)
	lc := &mockLifecycle{fail: errors.New("lifecycle boom")}
	cfg := common.DefaultConfig()
	cfg.RPCKey = key
	tr := transport.NewTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })
	srv, err := New(tr, lc, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	auth := hex.EncodeToString(key)

	resp, _ := doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/resume", auth, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("resume error status = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/pause", auth, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("pause error status = %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, http.MethodPost, ts.URL+"/v1/lifecycle/refresh-paths", auth, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("refresh error status = %d", resp.StatusCode)
	}
}

func TestLifecycleNotConfigured(t *testing.T) {
	srv, key := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()
	auth := hex.EncodeToString(key)
	for _, path := range []string{"/v1/lifecycle/resume", "/v1/lifecycle/pause", "/v1/lifecycle/refresh-paths"} {
		resp, _ := doJSON(t, http.MethodPost, ts.URL+path, auth, nil)
		if resp.StatusCode != http.StatusNotImplemented {
			t.Fatalf("%s status = %d, want 501", path, resp.StatusCode)
		}
	}
}

func TestLoadOrCreateIdentity(t *testing.T) {
	id, err := loadOrCreateIdentity("")
	if err != nil || id == nil {
		t.Fatalf("ephemeral: %v", err)
	}

	path := filepath.Join(t.TempDir(), "id")
	created, err := loadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("identity file missing: %v", err)
	}
	loaded, err := loadOrCreateIdentity(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if created.GetHexHash() != loaded.GetHexHash() {
		t.Fatalf("hash mismatch create=%s load=%s", created.GetHexHash(), loaded.GetHexHash())
	}
}

func TestAnnounceBridgeFilters(t *testing.T) {
	b := &announceBridge{}
	if got := b.AspectFilter(); len(got) != 1 || got[0] != "*" {
		t.Fatalf("AspectFilter = %v", got)
	}
	if !b.ReceivePathResponses() {
		t.Fatal("ReceivePathResponses should be true")
	}
}

func TestForgetResponse(t *testing.T) {
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	sess := newSession("s1", id)
	ch := sess.awaitResponse("deadbeef")
	sess.forgetResponse("deadbeef")
	if sess.deliverResponse("deadbeef", []byte("x")) {
		t.Fatal("forgotten waiter should not receive")
	}
	select {
	case <-ch:
		t.Fatal("channel should not receive after forget")
	default:
	}
}
