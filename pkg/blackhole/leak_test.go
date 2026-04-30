// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package blackhole

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

// TestBlackholeNoGoroutineLeak repeatedly creates tables, persists them,
// loads them back, and sweeps expired entries. The blackhole package itself
// owns no long-running goroutines; this test guards against an accidental
// regression that might add one.
func TestBlackholeNoGoroutineLeak(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		dir := t.TempDir()
		newLocal(t)
		tab := New(dir)
		id := bytes.Repeat([]byte{byte(i + 1)}, HashLen)
		if _, err := tab.Add(id, float64(time.Now().Add(time.Second).Unix()), "x"); err != nil {
			t.Fatalf("add: %v", err)
		}
		_ = tab.SweepExpired()
		if err := tab.LoadAll(); err != nil {
			t.Fatalf("load: %v", err)
		}
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()
	if final > baseline+5 {
		t.Errorf("possible goroutine leak: baseline=%d final=%d", baseline, final)
	}
}
