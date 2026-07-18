// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live mesh checks against public directory peers.
// Set RUN_LIVE_INTEROP=1. Optional: INTEROP_DIRECTORY_URL.

package interop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/destination"
	"quad4/reticulum-go/pkg/identity"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/transport"
)

const defaultDirectoryURL = "https://directory.rns.recipes/api/directory/submitted?search=&type=&status=online"

type directoryAPIEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TypeName string `json:"typeName"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Status   string `json:"status"`
	Network  string `json:"network"`
}

type directoryAPIResponse struct {
	Data []directoryAPIEntry `json:"data"`
}

func directoryURL() string {
	if u := strings.TrimSpace(os.Getenv("INTEROP_DIRECTORY_URL")); u != "" {
		return u
	}
	return defaultDirectoryURL
}

func fetchOnlineClearnetTCPPeers(t *testing.T, limit int) []directoryPeer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, directoryURL(), nil)
	if err != nil {
		t.Fatalf("directory request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("directory fetch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("directory status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		t.Fatalf("directory body: %v", err)
	}
	var parsed directoryAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("directory json: %v", err)
	}
	var peers []directoryPeer
	for _, e := range parsed.Data {
		if e.Status != "online" || e.Network != "clearnet" {
			continue
		}
		if e.Type != "tcp" && e.TypeName != "TCPClientInterface" {
			continue
		}
		if e.Host == "" || e.Port <= 0 {
			continue
		}
		if strings.Contains(e.Host, ":") {
			continue
		}
		name := e.Name
		if name == "" {
			name = fmt.Sprintf("%s:%d", e.Host, e.Port)
		}
		peers = append(peers, directoryPeer{Name: name, Host: e.Host, Port: e.Port})
		if limit > 0 && len(peers) >= limit {
			break
		}
	}
	if len(peers) == 0 {
		t.Fatal("no online clearnet TCP peers from directory")
	}
	return peers
}

func startDirectoryTCPClient(t *testing.T, tr *transport.Transport, peer directoryPeer, outgoing bool) (*interfaces.TCPClientInterface, error) {
	t.Helper()
	name := "dir_" + strconv.Itoa(peer.Port) + "_" + strings.ReplaceAll(peer.Host, ".", "_")
	iface, err := interfaces.NewTCPClientInterface(name, peer.Host, peer.Port, false, false, true)
	if err != nil {
		return nil, err
	}
	iface.SetOutgoingAllowed(outgoing)
	if err := tr.RegisterInterface(name, iface); err != nil {
		return nil, err
	}
	if err := iface.Start(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			return iface, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = iface.Stop()
	return nil, fmt.Errorf("not online within timeout")
}

// TestLiveDirectoryOutgoingPolicy connects to a real directory peer and verifies
// outgoing=no blocks local announces while still allowing RX from the mesh.
func TestLiveDirectoryOutgoingPolicy(t *testing.T) {
	liveOrSkip(t)
	peers := fetchOnlineClearnetTCPPeers(t, 12)

	cfg := &common.ReticulumConfig{EnableTransport: true, ShareInstance: false}
	tr := transport.NewTransport(cfg)
	defer func() { _ = tr.Close() }()
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	var iface *interfaces.TCPClientInterface
	var peer directoryPeer
	for _, p := range peers {
		c, err := startDirectoryTCPClient(t, tr, p, false)
		if err != nil {
			t.Logf("skip peer %s (%s:%d): %v", p.Name, p.Host, p.Port, err)
			continue
		}
		iface = c
		peer = p
		break
	}
	if iface == nil {
		t.Fatal("could not connect to any directory TCP peer")
	}
	t.Logf("connected receive-only to %s (%s:%d)", peer.Name, peer.Host, peer.Port)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In|destination.Out, destination.Single, "live_outgoing", tr, "probe")
	if err != nil {
		t.Fatal(err)
	}

	txBefore := iface.GetTxBytes()
	txPktsBefore := iface.GetTxPackets()
	rxBefore := iface.GetRxBytes()

	err = dest.Announce(false, nil, nil)
	if err == nil {
		t.Fatal("Announce on receive-only should fail with no writable interfaces")
	}
	if !errors.Is(err, common.ErrDestAnnounceNoWritable) {
		t.Fatalf("Announce on receive-only: got %v, want ErrDestAnnounceNoWritable", err)
	}
	time.Sleep(2 * time.Second)

	txAfter := iface.GetTxBytes()
	txPktsAfter := iface.GetTxPackets()
	if txAfter != txBefore || txPktsAfter != txPktsBefore {
		t.Fatalf("outgoing=no leaked TX: bytes %d->%d packets %d->%d on %s",
			txBefore, txAfter, txPktsBefore, txPktsAfter, peer.Name)
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if iface.GetRxBytes() > rxBefore {
			t.Logf("RX advanced %d->%d while TX stayed blocked", rxBefore, iface.GetRxBytes())
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("no inbound mesh traffic within wait (peer may be quiet) TX still blocked bytes=%d packets=%d",
		iface.GetTxBytes(), iface.GetTxPackets())
}

// TestLiveDirectoryAnnouncePath exercises announce + path discovery over a
// real directory TCP uplink with outgoing enabled (interop smoke).
func TestLiveDirectoryAnnouncePath(t *testing.T) {
	liveOrSkip(t)
	peers := fetchOnlineClearnetTCPPeers(t, 12)

	cfg := &common.ReticulumConfig{EnableTransport: true, ShareInstance: false}
	tr := transport.NewTransport(cfg)
	defer func() { _ = tr.Close() }()
	if err := tr.InitializePathRequestHandler(); err != nil {
		t.Fatalf("path handler: %v", err)
	}

	var iface *interfaces.TCPClientInterface
	var peer directoryPeer
	for _, p := range peers {
		c, err := startDirectoryTCPClient(t, tr, p, true)
		if err != nil {
			t.Logf("skip peer %s (%s:%d): %v", p.Name, p.Host, p.Port, err)
			continue
		}
		iface = c
		peer = p
		break
	}
	if iface == nil {
		t.Fatal("could not connect to any directory TCP peer")
	}
	t.Logf("connected outgoing to %s (%s:%d)", peer.Name, peer.Host, peer.Port)

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	dest, err := destination.New(id, destination.In|destination.Out, destination.Single, "live_dir", tr, "announce")
	if err != nil {
		t.Fatal(err)
	}

	txBefore := iface.GetTxBytes()
	if err := dest.Announce(false, nil, nil); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if iface.GetTxBytes() > txBefore {
			t.Logf("announce TX observed bytes %d->%d", txBefore, iface.GetTxBytes())
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("expected TX after announce on outgoing interface, still %d", iface.GetTxBytes())
}
