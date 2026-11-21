// SPDX-License-Identifier: LicenseRef-Reticulum
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
	monitorMu   sync.Mutex
	monitorStop chan struct{}
	running     bool
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
	// The monitor is a process singleton shared by every Node. Only the
	// first live Start owns the loop; a Stop frees the slot so the next
	// Start can monitor again.
	monitorMu.Lock()
	if monitorStop == nil {
		monitorStop = make(chan struct{})
	}
	ownsLoop := !running
	if ownsLoop {
		running = true
	}
	stop := monitorStop
	monitorMu.Unlock()

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
		if ownsLoop {
			monitorMu.Lock()
			running = false
			monitorMu.Unlock()
		}
		return
	}
	if !ownsLoop {
		return
	}
	go monitorLoop(ctx, interval, transport, stop)
}

// Stop ends periodic host probing.
func Stop() {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	running = false
	if monitorStop == nil {
		return
	}
	select {
	case <-monitorStop:
	default:
		close(monitorStop)
	}
	monitorStop = nil
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

func monitorLoop(ctx context.Context, interval time.Duration, transport bool, stop <-chan struct{}) {
	defer func() {
		monitorMu.Lock()
		running = false
		monitorMu.Unlock()
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var prev *Report
	for {
		select {
		case <-stop:
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
