// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"sync"
	"testing"
	"time"
)

func TestNewUDPInterfaceWithRetries(t *testing.T) {
	ui, err := NewUDPInterfaceWithRetries("u", "127.0.0.1:0", "", false, -1)
	if err != nil {
		t.Fatal(err)
	}
	if ui.maxReconnectTries != -1 {
		t.Fatalf("max tries = %d", ui.maxReconnectTries)
	}
}

func TestUDPReconnectStopRace(t *testing.T) {
	ui, err := NewUDPInterfaceWithRetries("u", "127.0.0.1:0", "", true, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = ui.Stop()
	}()
	time.Sleep(5 * time.Millisecond)
	wg.Wait()
}
