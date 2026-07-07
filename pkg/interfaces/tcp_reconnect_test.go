// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import "testing"

func TestNormalizeMaxReconnectTries(t *testing.T) {
	if got := NormalizeMaxReconnectTries(0); got != ReconnectWait*TCPProbesCount {
		t.Fatalf("default = %d, want %d", got, ReconnectWait*TCPProbesCount)
	}
	if got := NormalizeMaxReconnectTries(-1); got != -1 {
		t.Fatalf("unlimited = %d", got)
	}
	if got := NormalizeMaxReconnectTries(10); got != 10 {
		t.Fatalf("explicit = %d", got)
	}
}

func TestNewTCPClientInterfaceWithRetries(t *testing.T) {
	tc, err := NewTCPClientInterfaceWithRetries("t", "127.0.0.1", 1, false, false, false, -1)
	if err != nil {
		t.Fatal(err)
	}
	if tc.maxReconnectTries != -1 {
		t.Fatalf("max tries = %d", tc.maxReconnectTries)
	}
	_ = tc.Stop()
}
