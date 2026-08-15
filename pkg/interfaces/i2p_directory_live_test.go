// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/i2p"
)

const defaultI2PDirectoryURL = "https://directory.rns.recipes/api/directory/submitted?search=&type=i2p&status=online"

type i2pDirectoryEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TypeName string `json:"typeName"`
	Host     string `json:"host"`
	Status   string `json:"status"`
	Network  string `json:"network"`
}

type i2pDirectoryResponse struct {
	Data []i2pDirectoryEntry `json:"data"`
}

func i2pDirectoryURL() string {
	if u := strings.TrimSpace(os.Getenv("I2P_DIRECTORY_URL")); u != "" {
		return u
	}
	if u := strings.TrimSpace(os.Getenv("INTEROP_DIRECTORY_URL")); u != "" {
		return u
	}
	return defaultI2PDirectoryURL
}

func fetchOnlineI2PPeers(t *testing.T, limit int) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i2pDirectoryURL(), nil)
	if err != nil {
		t.Fatalf("directory request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("directory fetch failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("directory status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		t.Fatalf("directory body: %v", err)
	}
	var parsed i2pDirectoryResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("directory json: %v", err)
	}
	var hosts []string
	for _, e := range parsed.Data {
		if e.Status != "online" {
			continue
		}
		if e.Type != "i2p" && e.TypeName != "I2PInterface" {
			continue
		}
		host := strings.TrimSpace(strings.ToLower(e.Host))
		if host == "" || !strings.HasSuffix(host, ".b32.i2p") {
			continue
		}
		hosts = append(hosts, host)
		if limit > 0 && len(hosts) >= limit {
			break
		}
	}
	if len(hosts) == 0 {
		t.Skip("no online I2P peers from directory")
	}
	return hosts
}

func TestLiveI2PDirectoryPeerDial(t *testing.T) {
	liveI2POrSkip(t)
	peers := fetchOnlineI2PPeers(t, 8)
	ctrl := i2p.NewController(t.TempDir(), "")
	defer ctrl.Stop()

	deadline := time.Now().Add(3 * time.Minute)
	for _, dest := range peers {
		if time.Now().After(deadline) {
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		conn, sess, err := ctrl.DialStream(ctx, dest)
		cancel()
		if err != nil {
			t.Logf("dial %s: %v", dest, err)
			continue
		}
		_ = conn.Close()
		ctrl.ReleaseDialSession(sess)
		t.Logf("connected to directory peer %s", dest)
		return
	}
	t.Skip("no directory I2P peers reachable within budget")
}

func TestLiveI2PPeerStaysOfflineUntilConnect(t *testing.T) {
	liveI2POrSkip(t)
	dir := t.TempDir()
	parent, err := NewI2PInterface("i2p-offline-truth", &common.InterfaceConfig{
		Type:    "I2PInterface",
		Enabled: true,
	}, &FromConfigContext{
		I2PStoragePath: filepath.Join(dir, "storage"),
		TransportID:    []byte("offline-truth"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.controller.Stop()

	// Synthetic destination that will not have a LeaseSet.
	dest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.b32.i2p"
	peer := NewI2PInterfacePeer(parent, "offline-truth to "+dest, dest, 1, parent.cfg)
	defer peer.Stop()

	time.Sleep(3 * time.Second)
	if peer.IsOnline() {
		t.Fatal("peer must not be online before STREAM CONNECT succeeds")
	}
	if peer.LastError() == "" {
		t.Log("waiting briefly for dial error")
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) && peer.LastError() == "" {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if peer.IsOnline() {
		t.Fatal("peer became online unexpectedly")
	}
}
