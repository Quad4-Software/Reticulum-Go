// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"os"
	"runtime"
	runtimedebug "runtime/debug"
	"sync"
	"time"
)

// Heap memory belongs to the process, not to one Transport. Tests and
// simulations often run several transports in one process, so the trim clock
// is global. Otherwise each transport could schedule its own full GC.
var memTrimState = struct {
	sync.Mutex
	last        time.Time
	baselineKiB int64
}{last: time.Now(), baselineKiB: -1}

const (
	// memoryTrimFloor is Python MEM_TRIM_FLOOR: heap trimming never runs
	// more than once per minute.
	memoryTrimFloor = time.Minute
	// memoryTrimThrottle is Python mem_trim_throttle. Without RSS
	// introspection, trimming is limited to this timer.
	memoryTrimThrottle = 5 * time.Minute
	// memoryTrimGateKiB is Python mem_trim_gate_mb (8 MiB). RSS must grow
	// by at least this much from the last post-trim baseline before another
	// trim is due.
	memoryTrimGateKiB = 8 * 1024
)

func procRSSKiB() int64 {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1
	}
	start := 0
	for start < len(data) {
		end := start
		for end < len(data) && data[end] != '\n' {
			end++
		}
		line := data[start:end]
		if len(line) > len("VmRSS:") && string(line[:len("VmRSS:")]) == "VmRSS:" {
			var rss int64
			if _, err := fmt.Sscanf(string(line), "VmRSS: %d kB", &rss); err == nil {
				return rss
			}
			return -1
		}
		start = end + 1
	}
	return -1
}

// maybeTrimMemory returns idle Go heap to the operating system using the
// cadence Python RNS 1.6.1 applies to malloc_trim. FreeOSMemory performs a
// complete GC, so the hard one-minute floor and 8 MiB RSS-surplus gate are
// important: they keep this maintenance cheap on small or continuously busy
// instances while preventing large transient workloads from becoming the
// process's permanent RSS high-water mark.
func (t *Transport) maybeTrimMemory() {
	if t == nil {
		return
	}
	now := time.Now()
	rss := procRSSKiB()

	memTrimState.Lock()
	last := memTrimState.last
	baseline := memTrimState.baselineKiB

	due := false
	if now.Sub(last) < memoryTrimFloor {
		// The hard floor applies even when the RSS gate has been exceeded.
	} else if rss < 0 {
		if now.Sub(last) >= memoryTrimThrottle {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			due = ms.HeapSys > ms.HeapAlloc && (ms.HeapSys-ms.HeapAlloc)/1024 >= memoryTrimGateKiB
		}
	} else if baseline < 0 {
		due = true
	} else {
		due = rss-baseline >= memoryTrimGateKiB
	}
	if !due {
		memTrimState.Unlock()
		return
	}
	memTrimState.last = now
	memTrimState.Unlock()

	runtimedebug.FreeOSMemory()

	if after := procRSSKiB(); after >= 0 {
		memTrimState.Lock()
		memTrimState.baselineKiB = after
		memTrimState.Unlock()
	}
}
