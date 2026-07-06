// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/transport"
)

// freeTCPPort finds an ephemeral port by binding to :0 and releasing it
// immediately; controlapi.Server needs a concrete port up front rather than
// picking one itself.
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
	cfg.RPCKey = []byte("controlapi-e2e-test-key")
	cfg.EnableControlAPI = true
	cfg.ControlAPIHost = "127.0.0.1"
	cfg.ControlAPIPort = freeTCPPort(t)

	tr := transport.NewTransport(cfg)
	defer tr.Close()

	r := &Reticulum{config: cfg, transport: tr}
	r.StartControlAPI()
	if r.controlAPI == nil {
		t.Fatal("StartControlAPI did not set r.controlAPI")
	}
	defer r.controlAPI.Close()

	url := fmt.Sprintf("http://%s:%d/v1/health", cfg.ControlAPIHost, cfg.ControlAPIPort)
	token := hex.EncodeToString(cfg.RPCKey)

	var resp *http.Response
	var err error
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
