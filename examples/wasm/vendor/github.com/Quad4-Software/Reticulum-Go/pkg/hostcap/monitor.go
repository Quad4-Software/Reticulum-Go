// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package hostcap

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	lastReport  atomic.Pointer[Report]
	monitorStop chan struct{}
	monitorOnce sync.Once
)

// LastReport returns the most recent probe, or nil.
func LastReport() *Report {
	return lastReport.Load()
}

// Start runs an initial probe and optional periodic re-probes.
// transport is true when the node relays traffic for others.
func Start(ctx context.Context, transport bool) {
	if Skipped() {
		return
	}
	monitorOnce.Do(func() {
		monitorStop = make(chan struct{})
	})
	run := func() {
		if ctx == nil {
			ctx = context.Background()
		}
		r := Probe(ctx, defaultProbeDuration)
		r.Transport = transport
		storeReport(r)
		Log(r)
	}
	run()

	interval := probeInterval()
	if interval <= 0 {
		return
	}
	go monitorLoop(ctx, interval, transport)
}

// Stop ends periodic host probing.
func Stop() {
	if monitorStop == nil {
		return
	}
	select {
	case <-monitorStop:
	default:
		close(monitorStop)
	}
}

func storeReport(r Report) {
	cp := r
	lastReport.Store(&cp)
}

func probeInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("RETICULUM_HOST_PROBE_INTERVAL"))
	if v == "" {
		return 15 * time.Minute
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func monitorLoop(ctx context.Context, interval time.Duration, transport bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var prev *Report
	for {
		select {
		case <-monitorStop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			r := Probe(ctx, defaultProbeDuration)
			r.Transport = transport
			storeReport(r)
			if prev == nil || degraded(*prev, r) {
				Log(r)
			}
			cp := r
			prev = &cp
		}
	}
}

func degraded(before, after Report) bool {
	return classRank(after.MemClass) > classRank(before.MemClass) ||
		classRank(after.CPUClass) > classRank(before.CPUClass)
}

func classRank(c Class) int {
	return int(c)
}
