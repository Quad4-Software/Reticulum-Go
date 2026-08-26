// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package hostcap probes host CPU and memory copy throughput and logs when
// the node may be hardware-limited. Transport nodes with IFAC and high MTU
// interfaces copy large packets often and are sensitive to memory bandwidth.
package hostcap

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"
)

// Class rates a probe result.
type Class int

const (
	ClassOK Class = iota
	ClassWarn
	ClassError
)

// Report holds one host capability probe.
type Report struct {
	MemCopyGBps float64
	CPUMbps     float64
	NumCPU      int
	GOMAXPROCS  int
	MemClass    Class
	CPUClass    Class
	Transport   bool
	Duration    time.Duration
}

const (
	defaultProbeDuration = 100 * time.Millisecond
	copyChunkBytes       = 1 << 20
	cpuChunkBytes        = 65536
)

// Probe measures memory copy and CPU throughput for up to d.
// When d <= 0, defaultProbeDuration is used.
func Probe(ctx context.Context, d time.Duration) Report {
	if d <= 0 {
		d = defaultProbeDuration
	}
	start := time.Now()
	r := Report{
		NumCPU:     runtime.NumCPU(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		Duration:   d,
	}
	r.MemCopyGBps = measureMemCopy(ctx, d)
	r.CPUMbps = measureCPU(ctx, d)
	r.MemClass, r.CPUClass = classify(r.MemCopyGBps, r.CPUMbps)
	r.Duration = time.Since(start)
	return r
}

// Skipped reports whether host probing is disabled by environment.
func Skipped() bool {
	v := strings.TrimSpace(os.Getenv("RETICULUM_SKIP_HOST_PROBE"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func measureMemCopy(ctx context.Context, d time.Duration) float64 {
	src := make([]byte, copyChunkBytes)
	dst := make([]byte, copyChunkBytes)
	for i := range src {
		src[i] = byte(i)
	}
	deadline := time.Now().Add(d)
	var total uint64
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		copy(dst, src)
		total += copyChunkBytes
	}
	elapsed := d.Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(total) / elapsed / 1e9
}

func measureCPU(ctx context.Context, d time.Duration) float64 {
	buf := make([]byte, cpuChunkBytes)
	for i := range buf {
		buf[i] = byte(i * 37)
	}
	deadline := time.Now().Add(d)
	var total uint64
	var x uint64
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			break
		}
		for i := range buf {
			x ^= uint64(buf[i]) * 131
			buf[i] = byte(x >> (i & 7))
		}
		total += cpuChunkBytes
	}
	elapsed := d.Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(total) / elapsed / 1e6
}
