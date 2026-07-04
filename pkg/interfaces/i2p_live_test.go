// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/i2p"
)

func liveI2POrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LIVE_I2P") != "1" {
		t.Skip("set RUN_LIVE_I2P=1 to run live I2P interface tests")
	}
	addr := i2p.SAMAddressFromEnv()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("SAM not reachable at %s: %v", addr, err)
	}
	_ = conn.Close()
}

func TestLiveI2PInterfaceConnectable(t *testing.T) {
	liveI2POrSkip(t)
	dir := t.TempDir()
	storage := filepath.Join(dir, "storage")

	iface, err := NewI2PInterface("i2p-live", &common.InterfaceConfig{
		Type:           "I2PInterface",
		Enabled:        true,
		I2PConnectable: true,
	}, &FromConfigContext{
		I2PStoragePath: storage,
		TransportID:    []byte("live-interface-transport"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := iface.Start(); err != nil {
		t.Fatal(err)
	}
	defer iface.Stop()

	deadline := time.Now().Add(2 * time.Minute)
	for iface.Base32() == "" && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	b32 := iface.Base32()
	if b32 == "" {
		t.Fatal("connectable interface did not publish b32 address")
	}
	t.Logf("published %s.b32.i2p", b32)

	ctrl := i2p.NewController(filepath.Join(storage, "i2p"), "")
	defer ctrl.Stop()
	clientTun, err := ctrl.StartClientTunnel(b32+".b32.i2p", 0)
	if err != nil {
		t.Fatalf("client tunnel: %v", err)
	}

	var conn net.Conn
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", clientTun.LocalAddr(), 10*time.Second)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if conn == nil {
		t.Fatal("timed out establishing TCP via published I2P destination")
	}
	peerDeadline := time.Now().Add(90 * time.Second)
	for iface.Clients() < 1 && time.Now().Before(peerDeadline) {
		time.Sleep(250 * time.Millisecond)
	}
	_ = conn.Close()
	if iface.Clients() < 1 {
		t.Fatal("expected spawned inbound peer after I2P connection")
	}
}
