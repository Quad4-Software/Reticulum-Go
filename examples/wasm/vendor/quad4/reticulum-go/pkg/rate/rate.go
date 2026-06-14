// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package rate implements rate-limiting and ingress-control primitives
package rate

import (
	"sync"
	"time"
)

// Limiter is a token-bucket rate limiter.
type Limiter struct {
	rate       float64
	capacity   float64
	lastUpdate time.Time
	allowance  float64
	mutex      sync.Mutex
}

// NewLimiter returns a Limiter with the given rate and capacity.
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

// AnnounceRateControl gates announce re-broadcasts on a per-destination basis
// using a target rate, a grace count, and a penalty period.
type AnnounceRateControl struct {
	rateTarget  float64
	rateGrace   int
	ratePenalty float64

	announceHistory map[string][]time.Time
	mutex           sync.RWMutex
}

// NewAnnounceRateControl returns a new AnnounceRateControl.
func NewAnnounceRateControl(target float64, grace int, penalty float64) *AnnounceRateControl {
	return &AnnounceRateControl{
		rateTarget:      target,
		rateGrace:       grace,
		ratePenalty:     penalty,
		announceHistory: make(map[string][]time.Time),
	}
}

// AllowAnnounce reports whether an announce for destHash is allowed.
// Returns true unconditionally when rateTarget <= 0.
func (arc *AnnounceRateControl) AllowAnnounce(destHash string) bool {
	if arc == nil || arc.rateTarget <= 0 {
		return true
	}

	arc.mutex.Lock()
	defer arc.mutex.Unlock()

	history := arc.announceHistory[destHash]
	now := time.Now()

	cutoff := now.Add(-24 * time.Hour)
	newHistory := make([]time.Time, 0, len(history))
	for _, t := range history {
		if t.After(cutoff) {
			newHistory = append(newHistory, t)
		}
	}
	history = newHistory

	if len(history) < arc.rateGrace {
		arc.announceHistory[destHash] = append(history, now)
		return true
	}

	lastAnnounce := history[len(history)-1]
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

// CleanupExpired drops history entries older than 24 hours.
func (arc *AnnounceRateControl) CleanupExpired() {
	if arc == nil {
		return
	}
	arc.mutex.Lock()
	defer arc.mutex.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	for hash, history := range arc.announceHistory {
		filtered := history[:0]
		for _, t := range history {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			delete(arc.announceHistory, hash)
		} else {
			arc.announceHistory[hash] = filtered
		}
	}
}

// IngressControlConfig captures the ic_* knobs from the reference interface
// configuration.
type IngressControlConfig struct {
	Enabled             bool
	NewTime             time.Duration
	BurstFreqNew        float64
	BurstFreq           float64
	BurstHold           time.Duration
	BurstPenalty        time.Duration
	MaxHeldAnnounces    int
	HeldReleaseInterval time.Duration
}

// NewIngressControlConfig returns the reference defaults.
func NewIngressControlConfig() IngressControlConfig {
	return IngressControlConfig{
		Enabled:             true,
		NewTime:             time.Duration(DefaultNewTime) * time.Second,
		BurstFreqNew:        DefaultBurstFreqNew,
		BurstFreq:           DefaultBurstFreq,
		BurstHold:           time.Duration(DefaultBurstHold) * time.Second,
		BurstPenalty:        time.Duration(DefaultBurstPenalty) * time.Second,
		MaxHeldAnnounces:    DefaultMaxHeldAnnounces,
		HeldReleaseInterval: time.Duration(DefaultHeldReleaseInterval) * time.Second,
	}
}

// IngressControl implements per-interface announce ingress control.
type IngressControl struct {
	cfg IngressControlConfig

	spawnedAt      time.Time
	arrivals       []time.Time
	heldQueue      []ingressHeld
	heldIndex      map[string]int
	burstActive    bool
	burstClearedAt time.Time
	lastReleaseAt  time.Time

	mutex sync.Mutex
}

type ingressHeld struct {
	hash string
	data []byte
}

// NewIngressControl constructs an IngressControl with default thresholds.
func NewIngressControl(enabled bool) *IngressControl {
	cfg := NewIngressControlConfig()
	cfg.Enabled = enabled
	return NewIngressControlWith(cfg)
}

// NewIngressControlWith constructs an IngressControl with explicit
// thresholds. Zero values fall back to defaults.
func NewIngressControlWith(cfg IngressControlConfig) *IngressControl {
	def := NewIngressControlConfig()
	if cfg.NewTime <= 0 {
		cfg.NewTime = def.NewTime
	}
	if cfg.BurstFreqNew <= 0 {
		cfg.BurstFreqNew = def.BurstFreqNew
	}
	if cfg.BurstFreq <= 0 {
		cfg.BurstFreq = def.BurstFreq
	}
	if cfg.BurstHold <= 0 {
		cfg.BurstHold = def.BurstHold
	}
	if cfg.BurstPenalty <= 0 {
		cfg.BurstPenalty = def.BurstPenalty
	}
	if cfg.MaxHeldAnnounces <= 0 {
		cfg.MaxHeldAnnounces = def.MaxHeldAnnounces
	}
	if cfg.HeldReleaseInterval <= 0 {
		cfg.HeldReleaseInterval = def.HeldReleaseInterval
	}
	return &IngressControl{
		cfg:       cfg,
		spawnedAt: time.Now(),
		heldIndex: make(map[string]int),
	}
}

// Enabled reports whether the control is enforcing limits.
func (ic *IngressControl) Enabled() bool {
	return ic != nil && ic.cfg.Enabled
}

// ProcessAnnounce returns true when the announce should be processed
// immediately, false when it has been queued or dropped. isNewDest must
// be true when the receiving transport has no path for the destination.
func (ic *IngressControl) ProcessAnnounce(announceHash string, announceData []byte, isNewDest bool) bool {
	if !ic.Enabled() {
		return true
	}

	ic.mutex.Lock()
	defer ic.mutex.Unlock()

	now := time.Now()
	ic.recordArrivalLocked(now)

	threshold := ic.cfg.BurstFreq
	if now.Sub(ic.spawnedAt) < ic.cfg.NewTime {
		threshold = ic.cfg.BurstFreqNew
	}
	freq := ic.currentFrequencyLocked(now)

	if freq > threshold {
		ic.burstActive = true
		ic.burstClearedAt = time.Time{}
	} else if ic.burstActive {
		if ic.burstClearedAt.IsZero() {
			ic.burstClearedAt = now
		} else if now.Sub(ic.burstClearedAt) >= ic.cfg.BurstHold {
			ic.burstActive = false
		}
	}

	if !ic.burstActive {
		return true
	}
	if !isNewDest {
		return true
	}

	if _, dup := ic.heldIndex[announceHash]; dup {
		return false
	}
	if len(ic.heldQueue) >= ic.cfg.MaxHeldAnnounces {
		return false
	}
	buf := make([]byte, len(announceData))
	copy(buf, announceData)
	ic.heldIndex[announceHash] = len(ic.heldQueue)
	ic.heldQueue = append(ic.heldQueue, ingressHeld{hash: announceHash, data: buf})
	return false
}

// ReleaseHeldAnnounce returns one queued announce when the burst is
// clear and the release interval has elapsed.
func (ic *IngressControl) ReleaseHeldAnnounce() (string, []byte, bool) {
	if ic == nil {
		return "", nil, false
	}
	ic.mutex.Lock()
	defer ic.mutex.Unlock()

	if len(ic.heldQueue) == 0 {
		return "", nil, false
	}
	now := time.Now()
	if ic.burstActive {
		return "", nil, false
	}
	if !ic.burstClearedAt.IsZero() && now.Sub(ic.burstClearedAt) < ic.cfg.BurstHold+ic.cfg.BurstPenalty {
		return "", nil, false
	}
	if !ic.lastReleaseAt.IsZero() && now.Sub(ic.lastReleaseAt) < ic.cfg.HeldReleaseInterval {
		return "", nil, false
	}

	entry := ic.heldQueue[0]
	ic.heldQueue = ic.heldQueue[1:]
	delete(ic.heldIndex, entry.hash)
	for i, h := range ic.heldQueue {
		ic.heldIndex[h.hash] = i
	}
	ic.lastReleaseAt = now
	return entry.hash, entry.data, true
}

// HeldCount returns the number of currently queued announces.
func (ic *IngressControl) HeldCount() int {
	if ic == nil {
		return 0
	}
	ic.mutex.Lock()
	defer ic.mutex.Unlock()
	return len(ic.heldQueue)
}

// InBurst reports whether the controller is currently in a burst.
func (ic *IngressControl) InBurst() bool {
	if ic == nil {
		return false
	}
	ic.mutex.Lock()
	defer ic.mutex.Unlock()
	return ic.burstActive
}

func (ic *IngressControl) recordArrivalLocked(now time.Time) {
	ic.arrivals = append(ic.arrivals, now)
	cutoff := now.Add(-ic.cfg.BurstHold)
	idx := 0
	for ; idx < len(ic.arrivals); idx++ {
		if !ic.arrivals[idx].Before(cutoff) {
			break
		}
	}
	if idx > 0 {
		ic.arrivals = ic.arrivals[idx:]
	}
}

func (ic *IngressControl) currentFrequencyLocked(now time.Time) float64 {
	if len(ic.arrivals) < burstSampleMinimum {
		return 0
	}
	window := now.Sub(ic.arrivals[0]).Seconds()
	if window < MinElapsedSeconds {
		window = MinElapsedSeconds
	}
	return float64(len(ic.arrivals)-1) / window
}

const burstSampleMinimum = 8

// Path-request burst defaults (match upstream 0.8.x).
const (
	DefaultPRBurstFreqNew = 3.0
	DefaultPRBurstFreq    = 8.0
	DefaultECPRFreq       = 5.0
)
