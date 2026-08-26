// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package hostcap

import (
	"context"
	"testing"
	"time"
)

func TestProbeReturnsPositive(t *testing.T) {
	r := Probe(context.Background(), 50*time.Millisecond)
	if r.MemCopyGBps <= 0 {
		t.Fatalf("memcopy_gbps=%v", r.MemCopyGBps)
	}
	if r.CPUMbps <= 0 {
		t.Fatalf("cpu_mbps=%v", r.CPUMbps)
	}
	if r.NumCPU <= 0 {
		t.Fatal("num_cpu=0")
	}
}

func TestClassifyThresholds(t *testing.T) {
	mem, cpu := classify(10, 800)
	if mem != ClassOK || cpu != ClassOK {
		t.Fatalf("fast host: mem=%d cpu=%d", mem, cpu)
	}
	mem, cpu = classify(2, 200)
	if mem != ClassWarn || cpu != ClassWarn {
		t.Fatalf("mid host: mem=%d cpu=%d", mem, cpu)
	}
	mem, cpu = classify(0.5, 50)
	if mem != ClassError || cpu != ClassError {
		t.Fatalf("slow host: mem=%d cpu=%d", mem, cpu)
	}
}

func TestMattersForTransport(t *testing.T) {
	r := Report{Transport: true, MemClass: ClassWarn, CPUClass: ClassOK}
	if !r.MattersForTransport() {
		t.Fatal("expected matter")
	}
	r.Transport = false
	if r.MattersForTransport() {
		t.Fatal("leaf should not matter")
	}
}

func TestProbeRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Probe(ctx, time.Second)
	if r.Duration > 500*time.Millisecond {
		t.Fatalf("probe ran too long: %v", r.Duration)
	}
}

func TestStoreAndLastReport(t *testing.T) {
	r := Report{MemCopyGBps: 12.34, CPUMbps: 567.8}
	storeReport(r)
	got := LastReport()
	if got == nil || got.MemCopyGBps != 12.34 {
		t.Fatalf("last=%v", got)
	}
}

func TestDegraded(t *testing.T) {
	before := Report{MemClass: ClassOK, CPUClass: ClassOK}
	after := Report{MemClass: ClassWarn, CPUClass: ClassOK}
	if !degraded(before, after) {
		t.Fatal("expected degraded")
	}
	if degraded(after, before) {
		t.Fatal("improvement is not degraded")
	}
}
