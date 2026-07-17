// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/node"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestControlAPIStartStop(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.RPCKey = []byte("controlapi-e2e-test-key")
	cfg.EnableControlAPI = true
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = freeTCPPort(t)

	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Stop()

	r := &Reticulum{Node: n, config: cfg}
	r.StartControlAPI()
	if r.controlAPI == nil {
		t.Fatal("StartControlAPI did not set r.controlAPI")
	}
	defer r.controlAPI.Close()

	url := fmt.Sprintf("http://%s:%d/v1/health", cfg.ControlAPIHost, cfg.ControlAPIPort)
	token := hex.EncodeToString(cfg.RPCKey)

	var resp *http.Response
	for range 50 {
		req, reqErr := http.NewRequest(http.MethodGet, url, nil)
		if reqErr != nil {
			t.Fatalf("new request: %v", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("health request never succeeded: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// TestControlAPISessionHTTPSmoke exercises session and request-handler HTTP
// routes on a real daemon. Full link.request / resource round-trips are
// covered in pkg/controlapi parity tests (pipe-linked dual servers).
func TestControlAPISessionHTTPSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	cfg.RPCKey = []byte("controlapi-e2e-session-key")
	cfg.EnableControlAPI = true
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = freeTCPPort(t)

	n, err := node.New(cfg)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	defer n.Stop()

	r := &Reticulum{Node: n, config: cfg}
	r.StartControlAPI()
	if r.controlAPI == nil {
		t.Fatal("StartControlAPI did not set r.controlAPI")
	}
	defer r.controlAPI.Close()

	base := fmt.Sprintf("http://%s:%d", cfg.ControlAPIHost, cfg.ControlAPIPort)
	token := hex.EncodeToString(cfg.RPCKey)

	waitHealthy(t, base+"/v1/health", token)

	sessBody := postJSON(t, base+"/v1/sessions", token, map[string]any{})
	sessionID, _ := sessBody["session_id"].(string)
	if sessionID == "" {
		t.Fatal("missing session_id")
	}

	destBody := postJSON(t, base+"/v1/sessions/"+sessionID+"/destinations", token, map[string]any{
		"app_name":      "e2e_smoke",
		"accepts_links": true,
	})
	destHash, _ := destBody["destination_hash"].(string)
	if destHash == "" {
		t.Fatal("missing destination_hash")
	}

	reqURL := fmt.Sprintf("%s/v1/sessions/%s/destinations/%s/requests", base, sessionID, destHash)
	status := postJSONStatus(t, reqURL, token, map[string]any{"path": "/ping"})
	if status != http.StatusCreated {
		t.Fatalf("register request handler status = %d", status)
	}

	delReq, err := http.NewRequest(http.MethodDelete, reqURL+"?path=/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	delReq.Header.Set("Authorization", "Bearer "+token)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE handler status = %d", delResp.StatusCode)
	}

	delSess, err := http.NewRequest(http.MethodDelete, base+"/v1/sessions/"+sessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delSess.Header.Set("Authorization", "Bearer "+token)
	delSessResp, err := http.DefaultClient.Do(delSess)
	if err != nil {
		t.Fatal(err)
	}
	delSessResp.Body.Close()
	if delSessResp.StatusCode != http.StatusNoContent && delSessResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE session status = %d", delSessResp.StatusCode)
	}
}

func waitHealthy(t *testing.T, url, token string) {
	t.Helper()
	var err error
	for range 50 {
		req, reqErr := http.NewRequest(http.MethodGet, url, nil)
		if reqErr != nil {
			t.Fatalf("new request: %v", reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		var resp *http.Response
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("health never succeeded: %v", err)
}

func postJSON(t *testing.T, url, token string, body map[string]any) map[string]any {
	t.Helper()
	status, decoded := postJSONRaw(t, url, token, body)
	if status < 200 || status >= 300 {
		t.Fatalf("POST %s status = %d", url, status)
	}
	return decoded
}

func postJSONStatus(t *testing.T, url, token string, body map[string]any) int {
	t.Helper()
	status, _ := postJSONRaw(t, url, token, body)
	return status
}

func postJSONRaw(t *testing.T, url, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return resp.StatusCode, decoded
}
