// SPDX-License-Identifier: 0BSD
// Copyright (c) 2024-2026 Sudo-Ivan / Quad4.io
package rate

import (
	"sync"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
)

// Limiter implements a token-bucket style rate limiter.
type Limiter struct {
	rate       float64
	capacity   float64
	lastUpdate time.Time
	allowance  float64
	mutex      sync.Mutex
}

// NewLimiter returns a new Limiter with the given rate and capacity.
func NewLimiter(rate float64, capacity float64) *Limiter {
	return &Limiter{
		rate:       rate,
		capacity:   capacity,
		lastUpdate: time.Now(),
		allowance:  capacity,
	}
}

// Allow returns true if a token is available and consumes it.
func (l *Limiter) Allow() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.lastUpdate)
	l.lastUpdate = now

	l.allowance += elapsed.Seconds() * l.rate
	if l.allowance > l.capacity {
		l.allowance = l.capacity
	}

	if l.allowance < AllowanceMinThreshold {
		return false
	}

	l.allowance -= AllowanceDecrement
	return true
}

// AnnounceRateControl handles per-destination announce rate limiting
type AnnounceRateControl struct {
	rateTarget  float64
	rateGrace   int
	ratePenalty float64

	announceHistory map[string][]time.Time // Maps dest hash to announce times
	mutex           sync.RWMutex
}

// NewAnnounceRateControl returns a new AnnounceRateControl with the given target, grace count, and penalty.
func NewAnnounceRateControl(target float64, grace int, penalty float64) *AnnounceRateControl {
	return &AnnounceRateControl{
		rateTarget:      target,
		rateGrace:       grace,
		ratePenalty:     penalty,
		announceHistory: make(map[string][]time.Time),
	}
}

// AllowAnnounce returns true if an announce is allowed for the given destination hash.
func (arc *AnnounceRateControl) AllowAnnounce(destHash string) bool {
	arc.mutex.Lock()
	defer arc.mutex.Unlock()

	history := arc.announceHistory[destHash]
	now := time.Now()

	// Cleanup old history entries
	cutoff := now.Add(-24 * time.Hour)
	newHistory := []time.Time{}
	for _, t := range history {
		if t.After(cutoff) {
			newHistory = append(newHistory, t)
		}
	}
	history = newHistory

	// Allow if within grace period
	if len(history) < arc.rateGrace {
		arc.announceHistory[destHash] = append(history, now)
		return true
	}

	// Check rate
	lastAnnounce := history[len(history)-common.ONE]
	waitTime := arc.rateTarget
	if len(history) > arc.rateGrace+HistoryGraceThreshold {
		waitTime += arc.ratePenalty
	}

	if now.Sub(lastAnnounce).Seconds() < waitTime {
		return false
	}

	arc.announceHistory[destHash] = append(history, now)
	return true
}

// IngressControl handles new destination announce rate limiting
type IngressControl struct {
	enabled             bool
	burstFreqNew        float64
	burstFreq           float64
	burstHold           time.Duration
	burstPenalty        time.Duration
	maxHeldAnnounces    int
	heldReleaseInterval time.Duration

	heldAnnounces map[string][]byte // Maps announce hash to announce data
	lastBurst     time.Time
	announceCount int
	mutex         sync.RWMutex
}

// NewIngressControl returns a new IngressControl; when enabled it rate-limits new-destination announces.
func NewIngressControl(enabled bool) *IngressControl {
	return &IngressControl{
		enabled:             enabled,
		burstFreqNew:        DefaultBurstFreqNew,
		burstFreq:           DefaultBurstFreq,
		burstHold:           time.Duration(DefaultBurstHold) * time.Second,
		burstPenalty:        time.Duration(DefaultBurstPenalty) * time.Second,
		maxHeldAnnounces:    DefaultMaxHeldAnnounces,
		heldReleaseInterval: time.Duration(DefaultHeldReleaseInterval) * time.Second,
		heldAnnounces:       make(map[string][]byte),
		lastBurst:           time.Now(),
	}
}

// ProcessAnnounce returns true if the announce is accepted; otherwise it may be held.
func (ic *IngressControl) ProcessAnnounce(announceHash string, announceData []byte, isNewDest bool) bool {
	if !ic.enabled {
		return true
	}

	ic.mutex.Lock()
	defer ic.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(ic.lastBurst)

	// Reset counter if enough time has passed
	if elapsed > ic.burstHold+ic.burstPenalty {
		ic.announceCount = common.ZERO
		ic.lastBurst = now
	}

	// Check burst frequency
	maxFreq := ic.burstFreq
	if isNewDest {
		maxFreq = ic.burstFreqNew
	}

	ic.announceCount++

	seconds := elapsed.Seconds()
	if seconds < MinElapsedSeconds {
		seconds = MinElapsedSeconds
	}
	burstFreq := float64(ic.announceCount) / seconds

	// Hold announce if burst frequency exceeded
	if burstFreq > maxFreq {
		if len(ic.heldAnnounces) < ic.maxHeldAnnounces {
			ic.heldAnnounces[announceHash] = announceData
		}
		return false
	}

	return true
}

// ReleaseHeldAnnounce returns one held announce if any, and reports success.
func (ic *IngressControl) ReleaseHeldAnnounce() (string, []byte, bool) {
	ic.mutex.Lock()
	defer ic.mutex.Unlock()

	// Return first held announce if any exist
	for hash, data := range ic.heldAnnounces {
		delete(ic.heldAnnounces, hash)
		return hash, data, true
	}

	return "", nil, false
}
