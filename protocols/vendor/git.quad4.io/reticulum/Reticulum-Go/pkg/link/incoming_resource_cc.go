// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package link

import (
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/resource"
)

// Adaptive receive-side congestion control for incoming Resources.
//
// Python RNS (Resource.py) classifies the link from the measured data rate
// of each request round and picks a fixed window ceiling per class
// (WINDOW_MAX_VERY_SLOW=4, WINDOW_MAX_SLOW=10, WINDOW_MAX_FAST=75), and
// derives part timeouts from the expected in-flight rate (eifr) instead of
// a fixed timer. Reticulum-Go previously started every transfer at the
// fast ceiling with a fixed 250ms stall grace, which over-requests on slow
// links and re-requests too early (or too late) regardless of link speed.
//
// This controller combines both: rate-based round classification with
// hysteresis (Python), Go's gentler stall shrinking and faster window
// growth, and a stall grace derived from the smoothed RTT and measured
// data rate rather than a fixed constant. It changes only receiver-side
// pacing; the wire format and request semantics are unchanged.

// LinkClass is the measured quality class of the path carrying a Resource.
// It picks the request-window ceiling.
type LinkClass int

const (
	// ClassUnknown means no measurement has been made yet. Behaves like
	// ClassSlow until the first healthy round is classified.
	ClassUnknown LinkClass = iota
	// ClassVerySlow: sustained rounds under resource.RateVerySlow (250 B/s).
	// Ceiling resource.WindowMaxVerySlow (4), like Python WINDOW_MAX_VERY_SLOW.
	ClassVerySlow
	// ClassSlow: the default. Ceiling resource.WindowMaxSlow (10),
	// like Python WINDOW_MAX_SLOW.
	ClassSlow
	// ClassFast: sustained rounds above resource.RateFast (6250 B/s).
	// Ceiling resource.WindowMaxFast (75), like Python WINDOW_MAX_FAST.
	ClassFast
)

func (c LinkClass) String() string {
	switch c {
	case ClassVerySlow:
		return "very_slow"
	case ClassSlow:
		return "slow"
	case ClassFast:
		return "fast"
	default:
		return "unknown"
	}
}

const (
	// ccFastRoundsToPromote is how many consecutive fast rounds promote the
	// ceiling to WindowMaxFast (Python FAST_RATE_THRESHOLD rounds).
	ccFastRoundsToPromote = resource.FastRateThreshold

	// ccVerySlowRoundsToDemote is how many consecutive very-slow rounds
	// demote the ceiling to WindowMaxVerySlow (Python
	// VERY_SLOW_RATE_THRESHOLD rounds).
	ccVerySlowRoundsToDemote = resource.VerySlowRateThreshold

	// ccRTTEwmaAlpha is the smoothing step for the round-trip estimate,
	// matching Python's 5% rtt adjustment.
	ccRTTEwmaAlpha = 0.05

	// ccRateEwmaAlpha smooths the per-round data rate so one bursty round
	// cannot flip the link class.
	ccRateEwmaAlpha = 0.25

	// ccGraceFloor is the minimum stall grace (the previous fixed
	// incomingResourceStallGrace). Fast links must not wait longer than
	// this once the rate estimate says the window should have arrived.
	ccGraceFloor = 250 * time.Millisecond

	// ccGraceMax caps the derived grace so a pathological rate estimate
	// cannot park a transfer for minutes between re-requests.
	ccGraceMax = 120 * time.Second

	// ccUnknownExpectedMax bounds the assumed round time before any rate
	// has been measured. With the very-slow fallback rate this is ~25s of
	// transfer time, close to Python's initial conservative behavior.
	ccUnknownExpectedMax = 25 * time.Second

	// ccUnknownExpectedMin is the minimum assumed round time before any
	// rate has been measured, so HMU-only rounds on a fresh link do not
	// re-request within milliseconds of a slow peer's HMU.
	ccUnknownExpectedMin = 2 * time.Second

	// ccMarginFactor scales the expected round time into the grace. Twice
	// the expected time plus one RTT matches Python's
	// PART_TIMEOUT_FACTOR_AFTER_RTT plus latency allowance.
	ccMarginFactor = 2.0

	// ccReliableMarginFactor scales the margin further when the underlying
	// transport does its own loss recovery (TCP, QUIC, ...). Transport
	// retransmits legitimately delay parts beyond the app-level estimate.
	ccReliableMarginFactor = 2.0

	// ccPerStallBackoff is added to the grace per consecutive stall, capped
	// at ccPerStallBackoffMax, so repeated failures wait longer while a
	// single lost request is retried promptly.
	ccPerStallBackoff = 500 * time.Millisecond

	// ccPerStallBackoffMax caps the cumulative stall backoff.
	ccPerStallBackoffMax = 4 * time.Second
)

// resourceCC is the per-transfer controller state. All fields are guarded
// by the owning Link's incomingMu.
type resourceCC struct {
	class          LinkClass
	fastRounds     int
	verySlowRounds int

	// srtt is the smoothed round trip from request to first part, seconds.
	srtt float64
	// rateBps is the smoothed payload data rate, bytes per second.
	rateBps float64
	// knownRate is true once at least one round has been measured.
	knownRate bool

	roundStart     time.Time
	roundStartByte int64
	reqSentAt      time.Time
	reqAcked       bool
	roundHadStall  bool
	// classChanged is set by classify when the class moved; the transfer
	// applies the new ceiling to its window on the next request.
	classChanged bool
}

// classWindowMax returns the window ceiling for the current class.
func (cc *resourceCC) classWindowMax() int {
	switch cc.class {
	case ClassVerySlow:
		return resource.WindowMaxVerySlow
	case ClassFast:
		return resource.WindowMaxFast
	default:
		return resource.WindowMaxSlow
	}
}

// seed initializes controller state carried over from the previous
// completed transfer on the same link, plus the link's establishment RTT.
func (cc *resourceCC) seed(lastClass LinkClass, lastRateBps float64, lastRateKnown bool, linkRTT float64) {
	cc.class = lastClass
	if cc.class == ClassUnknown {
		cc.class = ClassSlow
	}
	cc.rateBps = lastRateBps
	cc.knownRate = lastRateKnown
	cc.srtt = linkRTT
}

// closeRound accounts a finished request round: from the previous request
// send (roundStart) to the next one (now). Rounds containing a stall or
// carrying too little data are skipped for rate purposes since their
// interval says nothing about link speed.
func (cc *resourceCC) closeRound(now time.Time, totalBytes int64) {
	defer func() {
		cc.roundStart = now
		cc.roundStartByte = totalBytes
		cc.roundHadStall = false
	}()
	if cc.roundStart.IsZero() || cc.roundHadStall {
		return
	}
	elapsed := now.Sub(cc.roundStart).Seconds()
	intervalBytes := totalBytes - cc.roundStartByte
	if elapsed <= 0 || intervalBytes < int64(resource.DefaultSegmentSize) {
		return
	}
	instRate := float64(intervalBytes) / elapsed
	if !cc.knownRate {
		cc.rateBps = instRate
		cc.knownRate = true
	} else {
		cc.rateBps += ccRateEwmaAlpha * (instRate - cc.rateBps)
	}
	cc.classify()
}

// classify updates the link class from the smoothed rate with Python's
// round counting: promotion after ccFastRoundsToPromote fast rounds
// (fast rounds persist across middle-band rounds, matching Python's
// fast_rate_rounds), demotion after ccVerySlowRoundsToDemote consecutive
// very-slow rounds.
func (cc *resourceCC) classify() {
	if !cc.knownRate {
		return
	}
	before := cc.class
	switch {
	case cc.rateBps > float64(resource.RateFast):
		cc.fastRounds++
		cc.verySlowRounds = 0
		if cc.fastRounds >= ccFastRoundsToPromote && cc.class != ClassFast {
			cc.class = ClassFast
		}
	case cc.rateBps < float64(resource.RateVerySlow):
		cc.fastRounds = 0
		cc.verySlowRounds++
		if cc.verySlowRounds >= ccVerySlowRoundsToDemote && cc.class != ClassVerySlow {
			cc.class = ClassVerySlow
		}
	default:
		// Middle band: neither fast nor very slow. Keep counting toward
		// promotion but reset the very-slow streak.
		cc.verySlowRounds = 0
	}
	if cc.class != before {
		cc.classChanged = true
	}
}

// takeClassChanged reports whether classify moved the class since the last
// call, so the caller can re-sync its window ceiling.
func (cc *resourceCC) takeClassChanged() bool {
	if cc.classChanged {
		cc.classChanged = false
		return true
	}
	return false
}

// noteRequestSent starts a new measurement round at now.
func (cc *resourceCC) noteRequestSent(now time.Time, totalBytes int64) {
	cc.roundStart = now
	cc.roundStartByte = totalBytes
	cc.roundHadStall = false
	cc.reqSentAt = now
	cc.reqAcked = false
}

// noteStallDuringRound records that the watchdog re-requested during the
// current round, invalidating its rate measurement.
func (cc *resourceCC) noteStallDuringRound() {
	cc.roundHadStall = true
}

// noteFirstPart samples the round-trip time from the outstanding request
// to its first accepted part. Later parts of the same round are ignored.
func (cc *resourceCC) noteFirstPart(now time.Time) {
	if cc.reqAcked || cc.reqSentAt.IsZero() {
		return
	}
	cc.reqAcked = true
	rtt := now.Sub(cc.reqSentAt).Seconds()
	if rtt < 0 {
		return
	}
	if cc.srtt <= 0 {
		cc.srtt = rtt
	} else {
		cc.srtt += ccRTTEwmaAlpha * (rtt - cc.srtt)
	}
}

// stallGrace returns how long the receiver should wait after the last
// progress before re-requesting. The grace tracks the expected arrival
// time of the outstanding window: outstanding bytes divided by the
// measured rate, scaled by a margin factor, plus one RTT, plus a small
// backoff per consecutive stall.
func (cc *resourceCC) stallGrace(outstandingBytes, hashmapBytes int64, waitingHmu, reliable bool, stalls int) time.Duration {
	expectedBytes := outstandingBytes
	if waitingHmu && hashmapBytes > expectedBytes {
		expectedBytes = hashmapBytes
	}
	var expected time.Duration
	if cc.knownRate && cc.rateBps > 0 {
		expected = time.Duration(float64(expectedBytes) / cc.rateBps * float64(time.Second))
	} else {
		// No rate measurement yet: scale from the smoothed RTT (the round
		// should take a handful of RTTs), bounded to stay responsive on
		// fast links and patient on slow ones. This mirrors Python's
		// rtt * TRAFFIC_TIMEOUT_FACTOR first-round timeout.
		expected = time.Duration(6 * cc.srtt * float64(time.Second))
		if expected < ccUnknownExpectedMin {
			expected = ccUnknownExpectedMin
		}
		if expected > ccUnknownExpectedMax {
			expected = ccUnknownExpectedMax
		}
	}
	margin := ccMarginFactor*expected.Seconds() + cc.srtt + ccGraceFloor.Seconds()
	if reliable {
		margin *= ccReliableMarginFactor
	}
	backoff := time.Duration(stalls) * ccPerStallBackoff
	if backoff > ccPerStallBackoffMax {
		backoff = ccPerStallBackoffMax
	}
	grace := time.Duration(margin*float64(time.Second)) + backoff
	if grace < ccGraceFloor {
		grace = ccGraceFloor
	}
	if grace > ccGraceMax {
		grace = ccGraceMax
	}
	return grace
}

// reliableTransportTypes lists interface types that perform their own
// in-order loss recovery, so application-level stall detection must allow
// extra time for transport retransmission.
var reliableTransportTypes = map[common.InterfaceType]bool{
	common.IFTypeTCP:          true,
	common.IFTypeUnix:         true,
	common.IFTypeQUIC:         true,
	common.IFTypeWebTransport: true,
	common.IFTypeHTTPS:        true,
	common.IFTypeBackbone:     true,
	common.IFTypeVSOCK:        true,
	common.IFTypeI2P:          true,
}

// transportReliable reports whether the interface carrying this link
// retransmits on its own.
func (l *Link) transportReliable() bool {
	iface := l.networkInterface
	if iface == nil {
		return false
	}
	return reliableTransportTypes[iface.GetType()]
}
