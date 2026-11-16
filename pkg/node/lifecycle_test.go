// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

func TestWatchDestinationAndRefreshPaths(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	hash := make([]byte, 16)
	hash[0] = 0xab
	if err := n.WatchDestination(hash); err != nil {
		t.Fatal(err)
	}
	if err := n.RefreshPathsWithDownDuration(time.Duration(transport.PathRequestTTL+1) * time.Second); err != nil {
		t.Fatal(err)
	}
	_ = n.Stop()
}

func TestOnNetworkLostPause(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.ShareInstance = false
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.OnNetworkLost(); err != nil {
		t.Fatal(err)
	}
	if !n.networkPaused {
		t.Fatal("expected paused")
	}
	_ = n.Stop()
}
