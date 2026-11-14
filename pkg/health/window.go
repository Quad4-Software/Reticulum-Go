// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package health

import (
	"math"
	"sync"
	"sync/atomic"
)

const (
	bucketSpanSec = int64(5)
	bucketsShort  = 12 // 60s / 5s
	bucketCount   = 60 // 300s / 5s
)

// windowedCounter tracks totals plus fixed-span buckets for rate windows.
// Hot path uses unix seconds only. No time.Time values are stored.
//
// add runs on every counted packet, so the common case (current bucket
// already active) is lock-free: atomics only. The mutex is taken solely to
// serialize bucket rollover, which happens at most once per 5s bucket.
type windowedCounter struct {
	mu         sync.Mutex
	total      atomic.Uint64
	lastBucket atomic.Int64 // bucket index, math.MinInt64 until first add
	buckets    [bucketCount]atomic.Uint64
}

func bucketIndexUnix(unixSec int64) int64 {
	return unixSec / bucketSpanSec
}

func bucketSlot(idx int64) int {
	m := int(idx % int64(bucketCount))
	if m < 0 {
		m += bucketCount
	}
	return m
}

func (w *windowedCounter) add(n uint64, unixSec int64) {
	cur := bucketIndexUnix(unixSec)
	if w.lastBucket.Load() == cur {
		w.buckets[bucketSlot(cur)].Add(n)
		w.total.Add(n)
		return
	}
	w.addSlow(cur, n)
}

// addSlow handles first use, bucket rollover, and stale timestamps. Caller
// must not hold w.mu.
func (w *windowedCounter) addSlow(cur int64, n uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advanceLocked(cur)
	w.buckets[bucketSlot(cur)].Add(n)
	w.total.Add(n)
}

// advanceLocked moves lastBucket forward to cur, clearing bucket slots that
// the rollover invalidates. Caller must hold w.mu. Buckets are zeroed before
// lastBucket is stored, so a lock-free add that observes the new lastBucket
// can never write into a slot that is about to be cleared.
func (w *windowedCounter) advanceLocked(cur int64) {
	lb := w.lastBucket.Load()
	if lb == math.MinInt64 {
		w.lastBucket.Store(cur)
		return
	}
	if cur <= lb {
		return
	}
	gap := cur - lb
	if gap >= int64(bucketCount) {
		for i := range w.buckets {
			w.buckets[i].Store(0)
		}
	} else {
		for b := lb + 1; b <= cur; b++ {
			w.buckets[bucketSlot(b)].Store(0)
		}
	}
	w.lastBucket.Store(cur)
}

func (w *windowedCounter) snapshot(unixSec int64) (total, rate60, rate300 uint64) {
	w.mu.Lock()
	w.advanceLocked(bucketIndexUnix(unixSec))
	w.mu.Unlock()
	total = w.total.Load()
	rate60 = w.sum(bucketIndexUnix(unixSec), bucketsShort)
	rate300 = w.sum(bucketIndexUnix(unixSec), bucketCount)
	return total, rate60, rate300
}

func (w *windowedCounter) sum(cur int64, nBuckets int) uint64 {
	if nBuckets > bucketCount {
		nBuckets = bucketCount
	}
	var sum uint64
	for i := 0; i < nBuckets; i++ {
		sum += w.buckets[bucketSlot(cur-int64(i))].Load()
	}
	return sum
}
