// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package main

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	runtimedebug "runtime/debug"
	"runtime/pprof"
	"sort"
	"strconv"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
)

// heapProfileKeep bounds how many heap profile files are retained in the
// storage directory.
const heapProfileKeep = 5

// memoryWatchInterval is how often the watcher samples heap usage.
const memoryWatchInterval = 60 * time.Second

// heapWarnFraction triggers the warning and optional dump when heap_alloc
// exceeds this share of the effective soft memory limit. The watcher
// re-arms once usage falls below heapRearmFraction.
const (
	heapWarnFraction  = 0.85
	heapRearmFraction = 0.70
)

// writeHeapProfileBounded writes a heap profile into dir and keeps only the
// newest heapProfileKeep heap-*.pprof files. Returns the path written.
func writeHeapProfileBounded(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "heap-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".pprof")
	f, err := os.Create(path) // #nosec G304 -- path is inside the configured storage dir
	if err != nil {
		return "", err
	}
	werr := pprof.WriteHeapProfile(f)
	cerr := f.Close()
	if werr != nil {
		return "", werr
	}
	if cerr != nil {
		return "", cerr
	}
	pruneHeapProfiles(dir)
	return path, nil
}

// pruneHeapProfiles removes heap-*.pprof files beyond heapProfileKeep,
// oldest first.
func pruneHeapProfiles(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "heap-*.pprof"))
	if err != nil || len(matches) <= heapProfileKeep {
		return
	}
	sort.Strings(matches)
	for _, stale := range matches[:len(matches)-heapProfileKeep] {
		_ = os.Remove(stale)
	}
}

// heapDumpOnThreshold reports whether threshold-triggered dumps are
// enabled via RETICULUM_HEAP_DUMP_ON_THRESHOLD.
func heapDumpOnThreshold() bool {
	switch os.Getenv("RETICULUM_HEAP_DUMP_ON_THRESHOLD") {
	case "1", "yes", "true", "on":
		return true
	}
	return false
}

// watchMemory samples heap usage and warns once per crossing when it
// exceeds heapWarnFraction of the effective soft limit. When
// RETICULUM_HEAP_DUMP_ON_THRESHOLD is set it also writes a bounded heap
// profile into the storage directory. No-op when no limit is in effect
// or dir is empty.
func watchMemory(dir string) {
	if dir == "" {
		return
	}
	if runtimedebug.SetMemoryLimit(-1) == math.MaxInt64 {
		return
	}
	go func() {
		armed := true
		ticker := time.NewTicker(memoryWatchInterval)
		defer ticker.Stop()
		for range ticker.C {
			limit := runtimedebug.SetMemoryLimit(-1)
			if limit <= 0 || limit == math.MaxInt64 {
				continue
			}
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			frac := float64(ms.HeapAlloc) / float64(limit)
			if frac >= heapWarnFraction && armed {
				armed = false
				debug.Log(debug.DebugWarning, "Heap usage near soft memory limit",
					"heap_alloc", ms.HeapAlloc, "limit", limit)
				if heapDumpOnThreshold() {
					if path, err := writeHeapProfileBounded(dir); err != nil {
						debug.Log(debug.DebugError, "Threshold heap dump failed", "error", err)
					} else {
						debug.Log(debug.DebugInfo, "Threshold heap profile written", "path", path)
					}
				}
			} else if frac < heapRearmFraction {
				armed = true
			}
		}
	}()
}

// memwatchStorageDir returns the transport storage directory for profile
// dumps, mirroring transportStoragePath.
func memwatchStorageDir(configPath string) string {
	if configPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(configPath), "storage")
}
