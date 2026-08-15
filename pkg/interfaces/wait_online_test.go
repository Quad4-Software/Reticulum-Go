// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"testing"
	"time"
)

func waitOnline(t *testing.T, iface Interface, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if iface.IsOnline() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s not online within %s", iface.GetName(), timeout)
}
