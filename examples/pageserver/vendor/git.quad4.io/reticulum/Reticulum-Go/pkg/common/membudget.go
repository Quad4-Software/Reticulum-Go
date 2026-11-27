// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
)

// ErrMemoryBudgetExceeded is returned when an in-memory allocation would
// exceed the configured soft budget. Callers may also treat this as ErrOOM
// via IsOOM.
var ErrMemoryBudgetExceeded = errors.New("in-memory storage budget exceeded")

// MemoryBudget tracks a soft byte budget for ephemeral staging (resources).
// It is safe for concurrent use. A limit of zero or negative means unlimited.
type MemoryBudget struct {
	used  atomic.Int64
	limit atomic.Int64
}

// NewMemoryBudget returns a budget with the given soft limit in bytes.
func NewMemoryBudget(limit int64) *MemoryBudget {
	b := &MemoryBudget{}
	b.limit.Store(limit)
	return b
}

// SetLimit updates the soft limit. Zero or negative disables the cap.
func (b *MemoryBudget) SetLimit(limit int64) {
	if b == nil {
		return
	}
	b.limit.Store(limit)
}

// Limit returns the configured soft limit (0 means unlimited).
func (b *MemoryBudget) Limit() int64 {
	if b == nil {
		return 0
	}
	return b.limit.Load()
}

// Used returns the number of bytes currently charged to the budget.
func (b *MemoryBudget) Used() int64 {
	if b == nil {
		return 0
	}
	return b.used.Load()
}

// TryReserve attempts to charge n bytes. Returns ErrMemoryBudgetExceeded when
// the reservation would exceed the limit. n <= 0 is a no-op.
func (b *MemoryBudget) TryReserve(n int64) error {
	if b == nil || n <= 0 {
		return nil
	}
	limit := b.limit.Load()
	for {
		used := b.used.Load()
		if limit > 0 && used > limit-n {
			return ErrMemoryBudgetExceeded
		}
		if b.used.CompareAndSwap(used, used+n) {
			return nil
		}
	}
}

// Release drops n bytes from the budget. n <= 0 is a no-op. Never goes negative.
func (b *MemoryBudget) Release(n int64) {
	if b == nil || n <= 0 {
		return
	}
	for {
		used := b.used.Load()
		next := max(used-n, 0)
		if b.used.CompareAndSwap(used, next) {
			return
		}
	}
}

// ApplySoftMemoryLimit installs Go soft heap limiting when limitBytes > 0.
// Returns the previous limit from debug.SetMemoryLimit.
func ApplySoftMemoryLimit(limitBytes int64) int64 {
	if limitBytes <= 0 {
		return debug.SetMemoryLimit(-1)
	}
	return debug.SetMemoryLimit(limitBytes)
}

// defaultSoftMemoryLimitFraction is the share of the detected memory
// ceiling used for the default soft heap limit.
const defaultSoftMemoryLimitFraction = 65

// DefaultSoftMemoryLimitBytes derives a default soft heap limit of about
// 65 percent of the cgroup memory limit when one is set, else of total
// system memory. Returns 0 when no ceiling can be determined, leaving the
// runtime default (unlimited).
func DefaultSoftMemoryLimitBytes() int64 {
	if lim := cgroupMemoryLimit(); lim > 0 {
		return lim * defaultSoftMemoryLimitFraction / 100
	}
	if lim := systemMemoryBytes(); lim > 0 {
		return lim * defaultSoftMemoryLimitFraction / 100
	}
	return 0
}

// ApplyDefaultSoftMemoryLimit installs a derived soft heap limit when the
// caller has not configured one: no soft_memory_limit config value, no
// GOMEMLIMIT environment setting, and no earlier programmatic limit. It
// reports the limit applied (0 when the runtime default stays in effect).
func ApplyDefaultSoftMemoryLimit() int64 {
	if os.Getenv("GOMEMLIMIT") != "" {
		return 0
	}
	if debug.SetMemoryLimit(-1) != math.MaxInt64 {
		return 0
	}
	lim := DefaultSoftMemoryLimitBytes()
	if lim <= 0 {
		return 0
	}
	debug.SetMemoryLimit(lim)
	return lim
}

// cgroupMemoryLimit reads the cgroup v2 or v1 memory ceiling. A v1
// sentinel near math.MaxInt64 means "no limit" and yields 0.
func cgroupMemoryLimit() int64 {
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
				return v
			}
		}
	}
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && v > 0 && v < math.MaxInt64/2 {
			return v
		}
	}
	return 0
}

// systemMemoryBytes reads MemTotal from /proc/meminfo. Returns 0 on any
// error, for example on non-Linux systems.
func systemMemoryBytes() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return kb * 1024
				}
			}
		}
	}
	return 0
}

// DefaultGCPercent is the aggressive collector target used by the daemon when
// the operator has not chosen GOGC. Tailscale uses the same policy: the packet
// path is close to allocation-free, so more frequent collection costs little
// compared with the memory saved in long-running network processes.
const DefaultGCPercent = 10

// ApplyDefaultGCPercent installs DefaultGCPercent unless GOGC is explicitly
// present in the environment. It reports whether the setting was applied.
func ApplyDefaultGCPercent() bool {
	if _, ok := os.LookupEnv("GOGC"); ok {
		return false
	}
	debug.SetGCPercent(DefaultGCPercent)
	return true
}
