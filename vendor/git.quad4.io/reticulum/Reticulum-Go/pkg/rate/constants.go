// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rate

// Default and threshold constants for rate limiting and announce control.
const (
	// DefaultRateTableRetainSec is the floor for per-destination announce
	// rate history retention, matching the reference RATE_TABLE_RETAIN.
	DefaultRateTableRetainSec = 3600.0

	DefaultNewTime             = 2 * 60 * 60
	DefaultBurstFreqNew        = 3.5
	DefaultBurstFreq           = 12.0
	DefaultBurstHold           = 60
	DefaultBurstPenalty        = 300
	DefaultMaxHeldAnnounces    = 256
	DefaultHeldReleaseInterval = 30

	AllowanceMinThreshold = 1.0
	AllowanceDecrement    = 1.0

	HistoryGraceThreshold = 1

	// MinElapsedSeconds is the minimum elapsed time used in burst frequency calculation to avoid division by zero.
	MinElapsedSeconds = 0.01
)
