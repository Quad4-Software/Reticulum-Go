// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package common

// StampsSettings carries the resolved RNS 1.6.0 stamp knobs (Transport.py
// STAMPS_* constants and [stamps] config values). The zero value is not the
// default; use DefaultStampsSettings.
type StampsSettings struct {
	Enabled           bool // STAMPS_ENABLED: validate stamps on announces/path requests
	Enforced          bool // STAMPS_ENFORCED: drop unstamped/invalid-stamp announces
	LREnforced        bool // LR_STAMPS_ENFORCED: reject under-stamped link requests
	StampDestinations bool // STAMP_DESTINATIONS: emit stamped destination announces
	StampPRs          bool // STAMP_PRS: emit stamped path requests
	StampLRs          bool // STAMP_LRS: emit stamped link requests
	AlwaysStampLRs    bool // ALWAYS_STAMP_LRS: stamp LRs even without a recalled cost

	DestinationCost  float64 // DESTINATION_COST, default 23.5
	DestinationValue float64 // DESTINATION_VALUE, default 25.0
	PRCost           float64 // PR_COST, default 15.8
	PRValue          float64 // PR_VALUE, default 15.8
	LRCost           float64 // LR_COST, default 12.5

	LinkStampTimeout float64 // LINK_STAMP_TIMEOUT seconds, default 15
	PRStampTimeout   float64 // PR_STAMP_TIMEOUT seconds, default 15
}

// DefaultStampsSettings returns the Python 1.6.0 defaults.
func DefaultStampsSettings() StampsSettings {
	return StampsSettings{
		Enabled:          true,
		StampLRs:         true,
		DestinationCost:  23.5,
		DestinationValue: 25.0,
		PRCost:           15.8,
		PRValue:          15.8,
		LRCost:           12.5,
		LinkStampTimeout: 15,
		PRStampTimeout:   15,
	}
}

// StampsConfig is the parsed [stamps] config section. Booleans only enable
// (matching Reticulum.py: a true value sets the flag; false/unset leaves the
// default). Float fields apply clamping on Apply.
type StampsConfig struct {
	EnforceStamps     bool
	StampDestinations bool
	StampPRs          bool
	AlwaysStampLRs    bool
	// StampLRs/StampLRsSet override the STAMP_LRS default (true) — there is
	// no stamp_lrs config key in Python; this is a runtime/test toggle.
	StampLRs    bool
	StampLRsSet bool

	DestinationCost     float64
	DestinationCostSet  bool
	DestinationValue    float64
	DestinationValueSet bool
	PRCost              float64
	PRCostSet           bool
	PRValue             float64
	PRValueSet          bool
	LRCost              float64
	LRCostSet           bool
	LRStampTimeout      float64
	LRStampTimeoutSet   bool
	PRStampTimeout      float64
	PRStampTimeoutSet   bool
}

// StampCostCap mirrors RNS.Transport.STAMP_COST_CAP (pkg/stamps keeps the
// primitive constant; common cannot import it without a cycle).
const StampCostCap = 100

// LinkCostCap mirrors RNS.Transport.LINK_COST_CAP.
const LinkCostCap = 40

func clampCost(v, cap float64) float64 {
	if v < 0 {
		return 0
	}
	if v > cap {
		return cap
	}
	return v
}

// Apply merges the parsed config onto base with Python clamping.
func (c StampsConfig) Apply(base StampsSettings) StampsSettings {
	s := base
	if c.EnforceStamps {
		s.Enforced = true
		s.LREnforced = true
	}
	if c.StampDestinations {
		s.StampDestinations = true
	}
	if c.StampPRs {
		s.StampPRs = true
	}
	if c.AlwaysStampLRs {
		s.AlwaysStampLRs = true
	}
	if c.StampLRsSet {
		s.StampLRs = c.StampLRs
	}
	if c.DestinationCostSet {
		s.DestinationCost = clampCost(c.DestinationCost, StampCostCap)
	}
	if c.DestinationValueSet {
		s.DestinationValue = clampCost(c.DestinationValue, StampCostCap)
	}
	if c.PRCostSet {
		s.PRCost = clampCost(c.PRCost, StampCostCap)
	}
	if c.PRValueSet {
		s.PRValue = clampCost(c.PRValue, StampCostCap)
	}
	if c.LRCostSet {
		s.LRCost = clampCost(c.LRCost, LinkCostCap)
	}
	if c.LRStampTimeoutSet && c.LRStampTimeout > 0.020 {
		s.LinkStampTimeout = c.LRStampTimeout
	} else if c.LRStampTimeoutSet {
		s.LinkStampTimeout = 0.020
	}
	if c.PRStampTimeoutSet && c.PRStampTimeout > 0.020 {
		s.PRStampTimeout = c.PRStampTimeout
	} else if c.PRStampTimeoutSet {
		s.PRStampTimeout = 0.020
	}
	return s
}

// StampsSettings resolves the effective stamp settings for this config.
// A nil config yields Python defaults.
func (c *ReticulumConfig) StampsSettings() StampsSettings {
	if c == nil {
		return DefaultStampsSettings()
	}
	return c.Stamps.Apply(DefaultStampsSettings())
}

// FilterConfig is the parsed [filter] section (RNS 1.6.0). Behaviour is
// applied by the transport announce path; this struct only carries values.
type FilterConfig struct {
	RequiredOnly    bool
	RequiredOnlySet bool
	IgnoredTypes    []string
	AllowedTypes    []string
	HopLimiter      int
	HopLimiterSet   bool
}
