// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestAutoInterfaceRescanOffline(t *testing.T) {
	cfg := &common.InterfaceConfig{Enabled: true}
	ai, err := NewAutoInterface("auto", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ai.RescanInterfaces(); err == nil {
		t.Fatal("expected error when offline")
	}
}

func TestAutoInterfaceWatchInterfacesFlag(t *testing.T) {
	cfg := &common.InterfaceConfig{Enabled: true}
	ai, err := NewAutoInterface("auto", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ai.SetWatchInterfaces(true)
	ai.Mutex.Lock()
	if !ai.watchInterfaces {
		t.Fatal("watch flag not set")
	}
	ai.lastRescan = time.Now()
	ai.maybeRescanLocked(time.Now())
	ai.Mutex.Unlock()
}
