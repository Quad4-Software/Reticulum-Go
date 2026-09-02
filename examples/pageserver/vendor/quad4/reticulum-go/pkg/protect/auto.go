// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"fmt"
	"time"
)

// AutoPhase is the runtime phase when configured mode is auto.
type AutoPhase int

const (
	// AutoLearning observes and does not block.
	AutoLearning AutoPhase = iota
	// AutoArmed enforces prevent after a stable learned baseline.
	AutoArmed
)

func (p AutoPhase) String() string {
	if p == AutoArmed {
		return "armed"
	}
	return "learning"
}

func (e *Engine) enforcementMode() Mode {
	if e == nil {
		return ModeOff
	}
	if e.mode == ModeAuto {
		if AutoPhase(e.autoPhase.Load()) == AutoArmed {
			return ModePrevent
		}
		return ModeDetect
	}
	return e.mode
}

// Phase returns the auto phase or a static stand-in for non-auto modes.
func (e *Engine) Phase() AutoPhase {
	if e == nil {
		return AutoLearning
	}
	if e.mode != ModeAuto {
		if e.mode == ModePrevent {
			return AutoArmed
		}
		return AutoLearning
	}
	return AutoPhase(e.autoPhase.Load())
}

// SetNetworkFingerprint updates the live mesh fingerprint.
// A change while armed forces relearn.
func (e *Engine) SetNetworkFingerprint(fp string) {
	if e == nil || fp == "" {
		return
	}
	e.mu.Lock()
	prev := e.fingerprint
	e.fingerprint = fp
	armed := e.mode == ModeAuto && AutoPhase(e.autoPhase.Load()) == AutoArmed
	e.mu.Unlock()
	if armed && prev != "" && prev != fp {
		e.beginRelearn("network")
	}
}

// NotifyInterfaces sets the fingerprint from the current interface name set.
func (e *Engine) NotifyInterfaces(names []string) {
	if e == nil {
		return
	}
	e.SetNetworkFingerprint(networkFingerprint(names))
}

func (e *Engine) beginRelearn(reason string) {
	if e == nil || e.mode != ModeAuto {
		return
	}
	e.autoPhase.Store(int32(AutoLearning))
	e.mu.Lock()
	e.learnStarted = e.now()
	e.stableWindows = 0
	e.driftWindows = 0
	e.promoted = false
	e.driftSec = 0
	e.driftSecMaxPPS = 0
	for _, st := range e.ifaces {
		st.adapt = adaptiveState{}
		st.adaptSec = 0
		st.adaptPeakPPS = 0
		st.adaptPeakBPS = 0
		st.tripAt = nil
		st.coolUntil = time.Time{}
	}
	e.mu.Unlock()
	e.warnAuto("relearn", reason)
	_ = e.Persist()
}

func (e *Engine) maybePromoteOrDrift(iface string, pps, bps float64) {
	if e == nil || e.mode != ModeAuto || e.disableAdaptive {
		return
	}
	now := e.now()
	phase := AutoPhase(e.autoPhase.Load())

	e.mu.Lock()
	st := e.ifaceLocked(iface)
	ready := st.adapt.ready
	ewmaPPS := st.adapt.ewmaPPS
	ewmaBPS := st.adapt.ewmaBPS
	totalSamples := 0
	readyIfaces := 0
	for _, s := range e.ifaces {
		totalSamples += s.adapt.samples
		if s.adapt.ready {
			readyIfaces++
		}
	}
	learnStarted := e.learnStarted
	if learnStarted.IsZero() {
		e.learnStarted = now
		learnStarted = now
	}
	minDur := e.autoLearnMinDuration
	minSamples := e.autoLearnMinSamples
	e.mu.Unlock()

	quiet := ready && ewmaPPS > 0 && pps <= ewmaPPS*1.5 && pps <= e.floorPPS &&
		(ewmaBPS <= 0 || bps <= ewmaBPS*1.5)

	if phase == AutoLearning {
		if !quiet {
			e.mu.Lock()
			e.stableWindows = 0
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.stableWindows++
		stable := e.stableWindows
		e.mu.Unlock()
		if totalSamples >= minSamples && readyIfaces > 0 &&
			now.Sub(learnStarted) >= minDur && stable >= AutoStableWindows {
			if e.transportNode && !e.transportPromoteReady() {
				return
			}
			e.promote()
		}
		return
	}

	// Armed: relearn on sustained moderately elevated traffic (new normal).
	// Sample at most once per second using that second's peak pps.
	if !ready || ewmaPPS <= 0 {
		return
	}
	upper := ewmaPPS * AutoDriftFactor * 3
	if floorCap := e.floorPPS * 2; floorCap > upper {
		upper = floorCap
	}
	e.mu.Lock()
	sec := now.Unix()
	if sec != e.driftSec {
		if e.driftSec != 0 {
			peak := e.driftSecMaxPPS
			if peak > ewmaPPS*AutoDriftFactor && peak <= upper {
				e.driftWindows++
			} else if peak <= ewmaPPS*1.5 {
				e.driftWindows = 0
			}
		}
		e.driftSec = sec
		e.driftSecMaxPPS = pps
	} else if pps > e.driftSecMaxPPS {
		e.driftSecMaxPPS = pps
	}
	dw := e.driftWindows
	e.mu.Unlock()
	if dw >= AutoDriftWindows {
		e.beginRelearn("drift")
	}
}

func (e *Engine) transportPromoteReady() bool {
	const minTripPPS = 80.0
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, st := range e.ifaces {
		if st == nil || !st.adapt.ready {
			continue
		}
		pps, _ := st.adapt.tripLine(e.maxPPS, e.maxBPS, e.floorPPS, e.floorBPS)
		if pps >= minTripPPS {
			return true
		}
	}
	return false
}

func (e *Engine) promote() {
	if e == nil || e.mode != ModeAuto {
		return
	}
	if !e.autoPhase.CompareAndSwap(int32(AutoLearning), int32(AutoArmed)) {
		return
	}
	e.mu.Lock()
	e.promoted = true
	e.stableWindows = 0
	e.driftWindows = 0
	e.mu.Unlock()
	e.warnAuto("promote", "stable")
	_ = e.Persist()
}

func (e *Engine) warnAuto(action, reason string) {
	if e == nil || e.warnWriter == nil {
		return
	}
	line := fmt.Sprintf("WARNING: dos_protection auto %s reason=%s phase=%s", action, reason, e.Phase().String())
	_, _ = fmt.Fprintln(e.warnWriter, line)
}

// Persist writes the learning store to disk.
func (e *Engine) Persist() error {
	if e == nil || e.storePath == "" {
		return nil
	}
	e.mu.Lock()
	st := &persistedStore{
		Fingerprint: e.fingerprint,
		Promoted:    e.mode == ModeAuto && AutoPhase(e.autoPhase.Load()) == AutoArmed,
		Ifaces:      make(map[string]persistedIface, len(e.ifaces)),
	}
	for name, is := range e.ifaces {
		if is.adapt.samples == 0 {
			continue
		}
		st.Ifaces[name] = persistedIface{
			EwmaPPS: is.adapt.ewmaPPS,
			EwmaBPS: is.adapt.ewmaBPS,
			Samples: is.adapt.samples,
			Ready:   is.adapt.ready,
		}
	}
	path := e.storePath
	e.mu.Unlock()
	return saveStore(path, st)
}

func (e *Engine) loadPersisted() {
	if e == nil || e.storePath == "" {
		return
	}
	st, err := loadStore(e.storePath)
	if err != nil || st == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fingerprint != "" && st.Fingerprint != "" && e.fingerprint != st.Fingerprint {
		// Network changed since last save. Start fresh in learning.
		e.learnStarted = e.now()
		e.autoPhase.Store(int32(AutoLearning))
		e.promoted = false
		return
	}
	if st.Fingerprint != "" && e.fingerprint == "" {
		e.fingerprint = st.Fingerprint
	}
	for name, row := range st.Ifaces {
		is := e.ifaceLocked(name)
		is.adapt.ewmaPPS = row.EwmaPPS
		is.adapt.ewmaBPS = row.EwmaBPS
		is.adapt.samples = row.Samples
		is.adapt.ready = row.Ready || row.Samples >= AdaptiveWarmupSamples
	}
	if e.mode == ModeAuto && st.Promoted && len(st.Ifaces) > 0 {
		e.autoPhase.Store(int32(AutoArmed))
		e.promoted = true
	}
	e.learnStarted = e.now()
}
