// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

// simPathfinderRW and simAnnounceRateKbps are optional overrides set only
// from _test.go helpers. nil keeps production PathfinderRW / AnnounceRateKbps.
var (
	simHooksMu          sync.Mutex
	simPathfinderRW     *float64
	simAnnounceRateKbps *float64
)

func effectivePathfinderRW() float64 {
	simHooksMu.Lock()
	rw := simPathfinderRW
	simHooksMu.Unlock()
	if rw != nil {
		return *rw
	}
	return PathfinderRW
}

func pathfinderRebroadcastDelay() time.Duration {
	rw := effectivePathfinderRW()
	if rw <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	windowMs := max(int64(rw*1000.0), 1)
	return time.Duration(int64(binary.BigEndian.Uint64(b[:])%uint64(windowMs))) * time.Millisecond // #nosec G115
}

func (t *Transport) announceRateAllow() bool {
	simHooksMu.Lock()
	bypass := simAnnounceRateKbps
	simHooksMu.Unlock()
	if bypass != nil && *bypass <= 0 {
		return true
	}
	return t.announceRate.Allow()
}

func simFastPathActive() bool {
	simHooksMu.Lock()
	rw := simPathfinderRW
	simHooksMu.Unlock()
	return rw != nil && *rw <= 0
}
