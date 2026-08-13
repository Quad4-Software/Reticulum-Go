// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import "time"

// Default thresholds and adaptive policy for mesh nodes.
const (
	// DefaultMaxPPS is the absolute ceiling for per-iface packet rate.
	DefaultMaxPPS = 2000.0

	// DefaultMaxBPS is the absolute ceiling for per-iface byte rate.
	DefaultMaxBPS = 2 * 1024 * 1024

	// DefaultFloorPPS is the minimum adaptive trip line while learning.
	DefaultFloorPPS = 50.0

	// DefaultFloorBPS is the minimum adaptive trip line while learning.
	DefaultFloorBPS = 32 * 1024

	// AdaptiveMultiplier trips when rate exceeds baseline times this factor.
	AdaptiveMultiplier = 4.0

	// AdaptiveWarmupSamples is how many window samples before adaptive trips apply.
	AdaptiveWarmupSamples = 8

	// EWMAAlpha blends new rate into the baseline (0..1).
	EWMAAlpha = 0.2

	// DefaultMaxConns caps concurrent accepted stream connections per iface.
	DefaultMaxConns = 256

	// DefaultMaxResources caps concurrent incoming resources process-wide.
	DefaultMaxResources = 64

	// DefaultMaxCrypto caps concurrent crypto jobs (decrypt verify HMAC).
	DefaultMaxCrypto = 256

	// DefaultMaxHandshake caps concurrent link handshake jobs.
	DefaultMaxHandshake = 32

	// BucketCount is the number of sub-buckets in the one-second rate window.
	BucketCount = 10

	// BucketDuration is the width of one rate sub-bucket.
	BucketDuration = 100 * time.Millisecond

	// WindowSeconds is the sliding window length for pps and bps.
	WindowSeconds = 1.0

	// CoolDownTripThreshold enters cool-down after this many trips in CoolDownTripWindow.
	CoolDownTripThreshold = 32

	// CoolDownTripWindow is the lookback for cool-down trip counting.
	CoolDownTripWindow = 5 * time.Second

	// CoolDownDuration is how long an iface stays in cool-down.
	CoolDownDuration = 15 * time.Second

	// WarnInterval is the minimum time between stdout warnings per key.
	WarnInterval = 5 * time.Second

	// MemorySampleInterval is how often the memory monitor samples heap.
	MemorySampleInterval = 2 * time.Second

	// MemoryTripRatio trips when HeapAlloc exceeds this fraction of the soft limit.
	MemoryTripRatio = 0.85

	// MemoryClearRatio clears the shed flag below this fraction of the soft limit.
	MemoryClearRatio = 0.70

	// DefaultHeapWatermarkBytes trips when no soft limit is set and heap exceeds this.
	DefaultHeapWatermarkBytes = 512 << 20

	// StoreFileName is the msgpack learning store under the storage directory.
	StoreFileName = "dos_protect.mpack"

	// StoreVersion is the persisted learning document version.
	StoreVersion = 1

	// AutoLearnMinSamples is quiet samples required before auto can promote.
	AutoLearnMinSamples = 24

	// AutoLearnMinDuration is wall time in learning before auto can promote.
	AutoLearnMinDuration = 60 * time.Second

	// AutoStableWindows is consecutive quiet windows needed to promote.
	AutoStableWindows = 12

	// AutoDriftFactor triggers relearn when quiet rate diverges this far from baseline.
	AutoDriftFactor = 3.0

	// AutoDriftWindows is consecutive divergent windows needed to relearn.
	AutoDriftWindows = 8

	// PersistInterval is how often learning state is flushed to disk.
	PersistInterval = 30 * time.Second

	// PeerBudgetFraction is the share of the interface's effective trip
	// line a single peer may consume before its own sub-bucket sheds,
	// independent of the interface-wide aggregate. Keeps one hostile peer
	// on a shared listener from exhausting the whole interface budget.
	PeerBudgetFraction = 0.5

	// MaxTrackedPeersPerIface bounds per-peer rate sub-buckets so the
	// mitigation itself cannot become an unbounded-memory DoS vector from
	// spoofed or churning source identities.
	MaxTrackedPeersPerIface = 4096

	// PeerIdleEvictAfter prunes a peer sub-bucket that has been quiet this
	// long, making room for new peers once MaxTrackedPeersPerIface is hit.
	PeerIdleEvictAfter = 5 * time.Minute
)
