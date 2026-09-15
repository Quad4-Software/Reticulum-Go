// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import "time"

// rateWindow tracks packets and bytes over a fixed one-second bucketed window.
// It avoids per-packet slice growth under floods.
type rateWindow struct {
	packets [BucketCount]uint32
	bytes   [BucketCount]uint64
	epoch   int64 // BucketDuration ticks since arbitrary zero
}

func (w *rateWindow) advance(now time.Time) {
	tick := now.UnixNano() / int64(BucketDuration)
	if w.epoch == 0 {
		w.epoch = tick
		return
	}
	delta := tick - w.epoch
	if delta <= 0 {
		return
	}
	if delta >= int64(BucketCount) {
		for i := range w.packets {
			w.packets[i] = 0
			w.bytes[i] = 0
		}
		w.epoch = tick
		return
	}
	for i := range delta {
		idx := int((w.epoch + 1 + i) % int64(BucketCount))
		w.packets[idx] = 0
		w.bytes[idx] = 0
	}
	w.epoch = tick
}

func (w *rateWindow) add(now time.Time, nbytes int) (pps float64, bps float64) {
	w.advance(now)
	idx := int(w.epoch % int64(BucketCount))
	w.packets[idx]++
	if nbytes > 0 {
		w.bytes[idx] += uint64(nbytes)
	}
	var pkts uint64
	var by uint64
	for i := range BucketCount {
		pkts += uint64(w.packets[i])
		by += w.bytes[i]
	}
	return float64(pkts) / WindowSeconds, float64(by) / WindowSeconds
}
