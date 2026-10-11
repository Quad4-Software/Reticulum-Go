// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"sync"
	"time"
)

const (
	// EndpointQuarantineConsecutiveFails quarantines an endpoint after this
	// many consecutive dial failures.
	EndpointQuarantineConsecutiveFails = 8
	// EndpointFlapWindow is the lookback window for flap bursts.
	EndpointFlapWindow = 10 * time.Minute
	// EndpointFlapBurst quarantines an endpoint after this many flaps inside
	// EndpointFlapWindow.
	EndpointFlapBurst = 6
	// EndpointFlapLifetime treats a connection that dies within this duration
	// of dialing as a flap.
	EndpointFlapLifetime = 20 * time.Second
	// EndpointQuarantineBase is the first quarantine duration. Repeated
	// quarantines double it up to EndpointQuarantineMax.
	EndpointQuarantineBase = 5 * time.Minute
	// EndpointQuarantineMax caps the escalating quarantine duration.
	EndpointQuarantineMax = time.Hour
	// endpointExpire drops tracker state after this much inactivity.
	endpointExpire = 24 * time.Hour
	// endpointMax caps the tracked endpoint count so a flood of distinct
	// addresses cannot grow the map without bound.
	endpointMax = 4096
)

// EndpointStatus is the operator-facing view of one endpoint's health.
// Tracked is false when the endpoint is not monitored.
type EndpointStatus struct {
	Tracked             bool
	DialFailures        uint64
	Flaps               uint64
	Successes           uint64
	Quarantined         bool
	QuarantineRemaining time.Duration
}

// EndpointTracker records dial outcomes per endpoint (host:port style keys)
// and suppresses dials into an escalating quarantine after sustained failure
// or flapping. Zero wire impact: this only shapes when this node dials out.
type EndpointTracker struct {
	mu  sync.Mutex
	eps map[string]*endpointState
	now func() time.Time
}

type endpointState struct {
	consecutiveFails int
	flapTimes        []time.Time
	quarantinedUntil time.Time
	quarantineCount  int
	lastActive       time.Time
	dialFailures     uint64
	flaps            uint64
	successes        uint64
}

// DefaultEndpointTracker is shared by all interfaces in the process so a
// flapping autoconnected endpoint counts across interfaces.
var DefaultEndpointTracker = NewEndpointTracker()

// NewEndpointTracker returns an empty tracker.
func NewEndpointTracker() *EndpointTracker {
	return &EndpointTracker{
		eps: make(map[string]*endpointState),
		now: time.Now,
	}
}

func (et *EndpointTracker) clock() time.Time {
	if et.now != nil {
		return et.now()
	}
	return time.Now()
}

func (et *EndpointTracker) get(endpoint string, now time.Time) *endpointState {
	st := et.eps[endpoint]
	if st == nil {
		if len(et.eps) >= endpointMax {
			return nil
		}
		st = &endpointState{}
		et.eps[endpoint] = st
	}
	st.lastActive = now
	return st
}

// prune drops idle, non-quarantined entries. Called under mu.
func (et *EndpointTracker) prune(now time.Time) {
	for k, st := range et.eps {
		if st.quarantinedUntil.After(now) {
			continue
		}
		if now.Sub(st.lastActive) > endpointExpire {
			delete(et.eps, k)
		}
	}
}

// DialAllowed reports whether a dial may proceed now. When false, the return
// duration is how long the caller should wait before trying again.
func (et *EndpointTracker) DialAllowed(endpoint string) (bool, time.Duration) {
	if et == nil || endpoint == "" {
		return true, 0
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	now := et.clock()
	et.prune(now)
	st := et.eps[endpoint]
	if st == nil {
		return true, 0
	}
	st.lastActive = now
	if st.quarantinedUntil.After(now) {
		return false, st.quarantinedUntil.Sub(now)
	}
	return true, 0
}

// RecordSuccess clears consecutive-failure state for the endpoint.
func (et *EndpointTracker) RecordSuccess(endpoint string) {
	if et == nil || endpoint == "" {
		return
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	now := et.clock()
	st := et.get(endpoint, now)
	if st == nil {
		return
	}
	st.successes++
	st.consecutiveFails = 0
	// Halve escalation on a good connect so a healthy endpoint recovers,
	// but keep history so chronic quarantine/release cycles still climb.
	st.quarantineCount /= 2
}

// RecordFailure counts a failed dial. At the consecutive-failure threshold
// the endpoint is quarantined with an escalating cooldown.
func (et *EndpointTracker) RecordFailure(endpoint string) {
	if et == nil || endpoint == "" {
		return
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	now := et.clock()
	st := et.get(endpoint, now)
	if st == nil {
		return
	}
	st.dialFailures++
	st.consecutiveFails++
	if st.consecutiveFails >= EndpointQuarantineConsecutiveFails {
		et.quarantineLocked(st, now)
	}
}

// RecordFlap counts a connection that dropped within EndpointFlapLifetime of
// connecting. A burst of flaps inside EndpointFlapWindow quarantines the
// endpoint the same way sustained dial failures do.
func (et *EndpointTracker) RecordFlap(endpoint string) {
	if et == nil || endpoint == "" {
		return
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	now := et.clock()
	st := et.get(endpoint, now)
	if st == nil {
		return
	}
	st.flaps++
	cutoff := now.Add(-EndpointFlapWindow)
	kept := st.flapTimes[:0]
	for _, ts := range st.flapTimes {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	st.flapTimes = append(kept, now)
	if len(st.flapTimes) >= EndpointFlapBurst {
		et.quarantineLocked(st, now)
	}
}

// quarantineLocked engages or escalates quarantine. Caller holds mu.
func (et *EndpointTracker) quarantineLocked(st *endpointState, now time.Time) {
	dur := EndpointQuarantineBase
	for i := 0; i < st.quarantineCount; i++ {
		dur *= 2
		if dur >= EndpointQuarantineMax {
			dur = EndpointQuarantineMax
			break
		}
	}
	st.quarantineCount++
	st.quarantinedUntil = now.Add(dur)
	st.consecutiveFails = 0
	st.flapTimes = st.flapTimes[:0]
}

// Status returns the operator view of an endpoint's health.
func (et *EndpointTracker) Status(endpoint string) EndpointStatus {
	if et == nil {
		return EndpointStatus{}
	}
	et.mu.Lock()
	defer et.mu.Unlock()
	now := et.clock()
	st := et.eps[endpoint]
	if st == nil {
		return EndpointStatus{}
	}
	s := EndpointStatus{
		Tracked:      true,
		DialFailures: st.dialFailures,
		Flaps:        st.flaps,
		Successes:    st.successes,
	}
	if st.quarantinedUntil.After(now) {
		s.Quarantined = true
		s.QuarantineRemaining = st.quarantinedUntil.Sub(now)
	}
	return s
}
