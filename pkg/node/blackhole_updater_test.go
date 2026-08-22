// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package node

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestStartBlackholeUpdaterNoSources(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	n.StartBlackholeUpdater()
	n.bhUpdaterMu.Lock()
	running := n.bhUpdaterRunning
	n.bhUpdaterMu.Unlock()
	if running {
		t.Fatal("updater should not start without sources")
	}
}

func TestStartBlackholeUpdaterIdempotent(t *testing.T) {
	cfg := common.DefaultConfig()
	cfg.BlackholeSources = [][]byte{bytesRepeat(0x11, 16)}
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Stop()
	n.StartBlackholeUpdater()
	n.StartBlackholeUpdater()
	n.bhUpdaterMu.Lock()
	running := n.bhUpdaterRunning
	n.bhUpdaterMu.Unlock()
	if !running {
		t.Fatal("expected updater running")
	}
	n.stopBlackholeUpdater()
}

func TestBlackholeUpdateIntervalDefault(t *testing.T) {
	cfg := common.DefaultConfig()
	n, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := n.blackholeUpdateInterval(); got != blackholeUpdaterDefaultIVL {
		t.Fatalf("interval=%s want %s", got, blackholeUpdaterDefaultIVL)
	}
	cfg.BlackholeUpdateInterval = 30 * time.Minute
	if got := n.blackholeUpdateInterval(); got != 30*time.Minute {
		t.Fatalf("interval=%s want 30m", got)
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
