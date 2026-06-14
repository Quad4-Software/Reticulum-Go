// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"sync"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/rate"
)

// ifaceState is per-interface announce cap and ingress/egress controllers.
type ifaceState struct {
	announceCap float64
	ingress     *rate.IngressControl
	egress      *rate.AnnounceRateControl
}

type ifaceStateTable struct {
	mu sync.RWMutex
	by map[string]*ifaceState
}

func newIfaceStateTable() *ifaceStateTable {
	return &ifaceStateTable{by: make(map[string]*ifaceState)}
}

func (t *ifaceStateTable) get(name string) *ifaceState {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.by[name]
}

func (t *ifaceStateTable) put(name string, st *ifaceState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.by[name] = st
}

func (t *ifaceStateTable) delete(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.by, name)
}

// snapshot returns name/state pairs for held-announce release (caller
// must not hold table lock).
func (t *ifaceStateTable) snapshot() []struct {
	name  string
	state *ifaceState
} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]struct {
		name  string
		state *ifaceState
	}, 0, len(t.by))
	for n, s := range t.by {
		out = append(out, struct {
			name  string
			state *ifaceState
		}{n, s})
	}
	return out
}

// buildIfaceState builds per-iface spam state; nil cfg uses defaults
// (announce_cap 2%, ingress on, rate thresholds from pkg/rate).
func buildIfaceState(cfg *common.InterfaceConfig) *ifaceState {
	st := &ifaceState{}

	st.announceCap = 2.0
	if cfg != nil && cfg.AnnounceCap > 0 {
		st.announceCap = cfg.AnnounceCap
	}

	if cfg != nil && cfg.AnnounceRateTarget > 0 {
		grace := cfg.AnnounceRateGrace
		if grace <= 0 {
			grace = rate.DefaultAnnounceRateGrace
		}
		st.egress = rate.NewAnnounceRateControl(
			cfg.AnnounceRateTarget,
			grace,
			cfg.AnnounceRatePenalty,
		)
	}

	icCfg := rate.NewIngressControlConfig()
	if cfg != nil {
		if cfg.IngressControlSet {
			icCfg.Enabled = cfg.IngressControl
		}
		if cfg.ICNewTime > 0 {
			icCfg.NewTime = time.Duration(cfg.ICNewTime) * time.Second
		}
		if cfg.ICBurstFreqNew > 0 {
			icCfg.BurstFreqNew = cfg.ICBurstFreqNew
		}
		if cfg.ICBurstFreq > 0 {
			icCfg.BurstFreq = cfg.ICBurstFreq
		}
		if cfg.ICMaxHeldAnnounces > 0 {
			icCfg.MaxHeldAnnounces = cfg.ICMaxHeldAnnounces
		}
		if cfg.ICBurstHold > 0 {
			icCfg.BurstHold = time.Duration(cfg.ICBurstHold) * time.Second
		}
		if cfg.ICBurstPenalty > 0 {
			icCfg.BurstPenalty = time.Duration(cfg.ICBurstPenalty) * time.Second
		}
		if cfg.ICHeldReleaseInterval > 0 {
			icCfg.HeldReleaseInterval = time.Duration(cfg.ICHeldReleaseInterval) * time.Second
		}
	}
	st.ingress = rate.NewIngressControlWith(icCfg)

	return st
}

// applyIfacePRConfig configures path-request burst thresholds on iface from cfg.
func applyIfacePRConfig(iface common.NetworkInterface, cfg *common.InterfaceConfig) {
	if iface == nil {
		return
	}
	if cfg != nil && cfg.IngressControlSet {
		iface.SetIngressControl(cfg.IngressControl)
	}
	icPrBurstFreqNew := rate.DefaultPRBurstFreqNew
	icPrBurstFreq := rate.DefaultPRBurstFreq
	ecPrFreq := rate.DefaultECPRFreq
	egressControl := false
	if cfg != nil {
		if cfg.ICPRBurstFreqNew > 0 {
			icPrBurstFreqNew = cfg.ICPRBurstFreqNew
		}
		if cfg.ICPRBurstFreq > 0 {
			icPrBurstFreq = cfg.ICPRBurstFreq
		}
		if cfg.ECPRFreq > 0 {
			ecPrFreq = cfg.ECPRFreq
		}
		if cfg.EgressControlSet {
			egressControl = cfg.EgressControl
		}
	}
	iface.SetPRBurstConfig(icPrBurstFreqNew, icPrBurstFreq, ecPrFreq, egressControl)
}
