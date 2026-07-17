// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package common

import (
	"errors"
	"runtime/debug"
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
