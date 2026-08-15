// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package health

import (
	"sync"
)

const (
	bucketSpanSec = int64(5)
	bucketsShort  = 12 // 60s / 5s
	bucketCount   = 60 // 300s / 5s
)

// windowedCounter tracks totals plus fixed-span buckets for rate windows.
// Hot path uses unix seconds only. No time.Time values are stored.
type windowedCounter struct {
	mu         sync.Mutex
	total      uint64
	lastBucket int64
	started    bool
	buckets    [bucketCount]uint64
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
	w.mu.Lock()
	cur := bucketIndexUnix(unixSec)
	w.advanceLocked(cur)
	w.buckets[bucketSlot(cur)] += n
	w.total += n
	w.mu.Unlock()
}

func (w *windowedCounter) snapshot(unixSec int64) (total, rate60, rate300 uint64) {
	w.mu.Lock()
	cur := bucketIndexUnix(unixSec)
	w.advanceLocked(cur)
	total = w.total
	rate60 = w.sumLocked(cur, bucketsShort)
	rate300 = w.sumLocked(cur, bucketCount)
	w.mu.Unlock()
	return total, rate60, rate300
}

func (w *windowedCounter) advanceLocked(cur int64) {
	if !w.started {
		w.lastBucket = cur
		w.started = true
		return
	}
	if cur <= w.lastBucket {
		return
	}
	gap := cur - w.lastBucket
	if gap >= int64(bucketCount) {
		clear(w.buckets[:])
		w.lastBucket = cur
		return
	}
	for b := w.lastBucket + 1; b <= cur; b++ {
		w.buckets[bucketSlot(b)] = 0
	}
	w.lastBucket = cur
}

func (w *windowedCounter) sumLocked(cur int64, nBuckets int) uint64 {
	if nBuckets > bucketCount {
		nBuckets = bucketCount
	}
	var sum uint64
	for i := 0; i < nBuckets; i++ {
		sum += w.buckets[bucketSlot(cur-int64(i))]
	}
	return sum
}
