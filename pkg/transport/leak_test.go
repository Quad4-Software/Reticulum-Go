// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package transport

import (
	"runtime"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

func TestTransportLeak(t *testing.T) {
	// Baseline goroutine count
	runtime.GC()
	baseline := runtime.NumGoroutine()

	cfg := &common.ReticulumConfig{}
	
	// Create and close many transport instances
	for i := 0; i < 100; i++ {
		tr := NewTransport(cfg)
		// Give it a tiny bit of time to start the goroutine
		time.Sleep(1 * time.Millisecond)
		tr.Close()
	}

	// Wait for goroutines to finish
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	
	final := runtime.NumGoroutine()
	
	// We allow a small margin for other system goroutines, 
	// but 100 leaks would be very obvious.
	if final > baseline+5 {
		t.Errorf("Potential goroutine leak: baseline %d, final %d", baseline, final)
	}
}

