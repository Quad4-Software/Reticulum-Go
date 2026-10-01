// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

//go:build linux

package interfaces

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
)

// TestNetwatchDebounceCoalesces verifies a burst of netlink events collapses
// into a single subscriber notification.
func TestNetwatchDebounceCoalesces(t *testing.T) {
	var fired atomic.Int32
	unsub := nlWatcher.subscribe(func() { fired.Add(1) })
	defer unsub()

	nlWatcher.noteEvent()
	nlWatcher.noteEvent()
	nlWatcher.noteEvent()

	deadline := time.Now().Add(netwatchDebounce + 2*time.Second)
	for time.Now().Before(deadline) && fired.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if fired.Load() == 0 {
		t.Fatal("netwatch notification never fired")
	}
	time.Sleep(200 * time.Millisecond)
	if n := fired.Load(); n != 1 {
		t.Fatalf("debounce fired %d times for one burst, want 1", n)
	}
}

// TestNetwatchUnsub verifies removed subscribers no longer receive events.
func TestNetwatchUnsub(t *testing.T) {
	var fired atomic.Int32
	unsub := nlWatcher.subscribe(func() { fired.Add(1) })
	unsub()
	unsub() // idempotent

	nlWatcher.noteEvent()
	time.Sleep(netwatchDebounce + 300*time.Millisecond)
	if fired.Load() != 0 {
		t.Fatal("unsubscribed callback still fired")
	}
}

// TestNetwatchManySubs verifies repeated subscribe/unsubscribe cycles do not
// grow the subscriber map. Other tests may legitimately hold live
// subscriptions, so growth is measured against a baseline.
func TestNetwatchManySubs(t *testing.T) {
	nlWatcher.mu.Lock()
	base := len(nlWatcher.subs)
	nlWatcher.mu.Unlock()

	var fired atomic.Int32
	for range 16 {
		unsub := nlWatcher.subscribe(func() { fired.Add(1) })
		unsub()
	}
	nlWatcher.mu.Lock()
	n := len(nlWatcher.subs)
	nlWatcher.mu.Unlock()
	if n != base {
		t.Fatalf("subscriber map grew: %d entries, baseline %d", n, base)
	}
}

// TestAutoNetworkChangeOffline verifies the netwatch callback is a safe no-op
// before the interface is online.
func TestAutoNetworkChangeOffline(t *testing.T) {
	ai, err := NewAutoInterface("autoOff", &common.InterfaceConfig{Enabled: true})
	if err != nil {
		t.Fatalf("NewAutoInterface: %v", err)
	}
	ai.onNetworkChange() // must not panic or rescan while offline
}
