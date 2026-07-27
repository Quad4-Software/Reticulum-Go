// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"sort"
)

// IfaceSnapshot is per-interface protect state for operators.
type IfaceSnapshot struct {
	Name            string  `json:"name" msgpack:"name"`
	EWMAPPS         float64 `json:"ewma_pps" msgpack:"ewma_pps"`
	EWMABPS         float64 `json:"ewma_bps" msgpack:"ewma_bps"`
	AdaptiveSamples int     `json:"adaptive_samples" msgpack:"adaptive_samples"`
	AdaptiveReady   bool    `json:"adaptive_ready" msgpack:"adaptive_ready"`
	TripLinePPS     float64 `json:"trip_line_pps" msgpack:"trip_line_pps"`
	TripLineBPS     float64 `json:"trip_line_bps" msgpack:"trip_line_bps"`
	InCoolDown      bool    `json:"in_cooldown" msgpack:"in_cooldown"`
	CoolDownSeconds float64 `json:"cooldown_seconds" msgpack:"cooldown_seconds"`
}

// TripCountsSnapshot aggregates process-wide trip counters.
type TripCountsSnapshot struct {
	PPS       uint64 `json:"pps" msgpack:"pps"`
	BPS       uint64 `json:"bps" msgpack:"bps"`
	Handler   uint64 `json:"handler" msgpack:"handler"`
	Conn      uint64 `json:"conn" msgpack:"conn"`
	Resource  uint64 `json:"resource" msgpack:"resource"`
	Memory    uint64 `json:"memory" msgpack:"memory"`
	Crypto    uint64 `json:"crypto" msgpack:"crypto"`
	Handshake uint64 `json:"handshake" msgpack:"handshake"`
	CoolDown  uint64 `json:"cooldown" msgpack:"cooldown"`
}

// Snapshot is the operator-facing protect state.
type Snapshot struct {
	Mode           string             `json:"mode" msgpack:"mode"`
	Phase          string             `json:"phase" msgpack:"phase"`
	Enforcement    string             `json:"enforcement" msgpack:"enforcement"`
	Fingerprint    string             `json:"fingerprint" msgpack:"fingerprint"`
	SheddingMemory bool               `json:"shedding_memory" msgpack:"shedding_memory"`
	TripCounts     TripCountsSnapshot `json:"trip_counts" msgpack:"trip_counts"`
	Ifaces         []IfaceSnapshot    `json:"ifaces" msgpack:"ifaces"`
}

// CurrentSnapshot builds a point-in-time view of the default engine.
func CurrentSnapshot() Snapshot {
	return Default().snapshot()
}

// snapshot builds a point-in-time view of e.
func (e *Engine) snapshot() Snapshot {
	if e == nil {
		return Snapshot{Mode: ModeOff.String(), Phase: AutoLearning.String(), Enforcement: ModeOff.String()}
	}
	out := Snapshot{
		Mode:           e.mode.String(),
		Phase:          e.Phase().String(),
		Enforcement:    e.enforcementMode().String(),
		SheddingMemory: e.Shedding(),
		TripCounts: TripCountsSnapshot{
			PPS:       e.TripCount(ReasonPPS),
			BPS:       e.TripCount(ReasonBPS),
			Handler:   e.TripCount(ReasonHandler),
			Conn:      e.TripCount(ReasonConn),
			Resource:  e.TripCount(ReasonResource),
			Memory:    e.TripCount(ReasonMemory),
			Crypto:    e.TripCount(ReasonCrypto),
			Handshake: e.TripCount(ReasonHandshake),
			CoolDown:  e.TripCount(ReasonCoolDown),
		},
	}
	now := e.now()
	e.mu.Lock()
	out.Fingerprint = e.fingerprint
	names := make([]string, 0, len(e.ifaces))
	for name := range e.ifaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		st := e.ifaces[name]
		if st == nil {
			continue
		}
		row := IfaceSnapshot{Name: name}
		row.EWMAPPS = st.adapt.ewmaPPS
		row.EWMABPS = st.adapt.ewmaBPS
		row.AdaptiveSamples = st.adapt.samples
		row.AdaptiveReady = st.adapt.ready
		row.TripLinePPS, row.TripLineBPS = st.adapt.tripLine(e.maxPPS, e.maxBPS, e.floorPPS, e.floorBPS)
		if !e.disableCoolDown && now.Before(st.coolUntil) {
			row.InCoolDown = true
			row.CoolDownSeconds = st.coolUntil.Sub(now).Seconds()
		}
		out.Ifaces = append(out.Ifaces, row)
	}
	e.mu.Unlock()
	return out
}

// ActivePressure reports whether armed prevent is shedding or any iface is in cool-down.
func (s Snapshot) ActivePressure() bool {
	if s.Mode == ModeOff.String() {
		return false
	}
	if s.Enforcement != ModePrevent.String() {
		return false
	}
	if s.SheddingMemory {
		return true
	}
	for _, row := range s.Ifaces {
		if row.InCoolDown {
			return true
		}
	}
	total := s.TripCounts.PPS + s.TripCounts.BPS + s.TripCounts.Handler +
		s.TripCounts.Conn + s.TripCounts.Crypto + s.TripCounts.Handshake + s.TripCounts.CoolDown
	return total > 0
}

// CoolDownActive is true when any iface snapshot reports cool-down.
func (s Snapshot) CoolDownActive() bool {
	for _, row := range s.Ifaces {
		if row.InCoolDown {
			return true
		}
	}
	return false
}
