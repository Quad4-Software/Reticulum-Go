// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "testing"

func TestNewUDPInterfaceWithRetries(t *testing.T) {
	ui, err := NewUDPInterfaceWithRetries("u", "127.0.0.1:0", "", false, -1)
	if err != nil {
		t.Fatal(err)
	}
	if ui.maxReconnectTries != -1 {
		t.Fatalf("max tries = %d", ui.maxReconnectTries)
	}
}
