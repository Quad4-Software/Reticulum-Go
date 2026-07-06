// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// simPathfinderRW and simAnnounceRateKbps are optional overrides set only
// from _test.go helpers. nil keeps production PathfinderRW / AnnounceRateKbps.
var simPathfinderRW *float64
var simAnnounceRateKbps *float64

func effectivePathfinderRW() float64 {
	if simPathfinderRW != nil {
		return *simPathfinderRW
	}
	return PathfinderRW
}

func pathfinderRebroadcastDelay() time.Duration {
	rw := effectivePathfinderRW()
	if rw <= 0 {
		return 0
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0
	}
	windowMs := max(int64(rw*1000.0), 1)
	return time.Duration(int64(binary.BigEndian.Uint64(b)%uint64(windowMs))) * time.Millisecond // #nosec G115
}

func (t *Transport) announceRateAllow() bool {
	if simAnnounceRateKbps != nil && *simAnnounceRateKbps <= 0 {
		return true
	}
	return t.announceRate.Allow()
}

func simFastPathActive() bool {
	return simPathfinderRW != nil && *simPathfinderRW <= 0
}
