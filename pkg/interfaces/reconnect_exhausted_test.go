// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestReconnectExhaustedStopsWithoutIdleRetry(t *testing.T) {
	stop := make(chan struct{})
	exhausted := make(chan struct{})
	var attempts int
	rd := newReconnectDriver("test", 2, stop, func() (net.Conn, error) {
		attempts++
		return nil, errors.New("dial failed")
	}, func(net.Conn) {})
	rd.setOnExhausted(func() { close(exhausted) })

	rd.start()
	select {
	case <-exhausted:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for exhausted callback")
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, want at least 2", attempts)
	}
}
