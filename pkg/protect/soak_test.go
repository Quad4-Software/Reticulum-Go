// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
)

// TestProtectSoakFlood sustains a prevent-mode flood and checks heap/goroutine budgets.
func TestProtectSoakFlood(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak in -short mode")
	}
	secs := 90
	if v := os.Getenv("SOAK_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 5 {
			t.Fatalf("SOAK_SECONDS=%q invalid", v)
		}
		secs = n
	}
	duration := time.Duration(secs) * time.Second

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	baseG := runtime.NumGoroutine()

	var buf bytes.Buffer
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          200,
		MaxBPS:          256 * 1024,
		WarnWriter:      &buf,
		WarnInterval:    time.Second,
		DisableCoolDown: true,
	})
	t.Cleanup(func() {
		e.StopMemoryMonitor()
		SetDefault(nil)
	})
	SetDefault(e)
	e.StartMemoryMonitor()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = e.AdmitPacket("soak0", 128)
			}
		}
	}()

	time.Sleep(duration)
	close(stop)
	<-done

	if e.TripCount(ReasonPPS) == 0 && e.TripCount(ReasonBPS) == 0 {
		t.Fatal("soak expected trips under prevent flood")
	}

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	heapDelta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	gDelta := runtime.NumGoroutine() - baseG
	const maxHeapDelta = 64 << 20
	const maxGoroutineDelta = 64
	if heapDelta > maxHeapDelta {
		t.Fatalf("heap delta %d exceeds budget %d", heapDelta, maxHeapDelta)
	}
	if gDelta > maxGoroutineDelta {
		t.Fatalf("goroutine delta %d exceeds budget %d", gDelta, maxGoroutineDelta)
	}
}

// TestProtectSoakDetectQuietTraffic checks detect mode does not trip on sparse traffic.
func TestProtectSoakDetectQuietTraffic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak in -short mode")
	}
	secs := 10
	if v := os.Getenv("SOAK_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 5 {
			secs = min(n, 30)
		}
	}
	var buf bytes.Buffer
	e := New(Options{
		Mode:         ModeDetect,
		MaxPPS:       DefaultMaxPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(time.Duration(secs) * time.Second)
	for {
		select {
		case <-deadline:
			if e.TripCount(ReasonPPS) != 0 || e.TripCount(ReasonBPS) != 0 {
				t.Fatalf("quiet traffic should not trip got pps=%d bps=%d", e.TripCount(ReasonPPS), e.TripCount(ReasonBPS))
			}
			return
		case <-ticker.C:
			_ = e.AdmitPacket("quiet0", 64)
		}
	}
}
