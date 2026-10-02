// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package protect

// adaptiveState learns a quiet baseline and derives dynamic trip lines.
type adaptiveState struct {
	ewmaPPS float64
	ewmaBPS float64
	samples int
	ready   bool
}

func (a *adaptiveState) observe(pps, bps float64) {
	if a.ready {
		// Only learn from near-baseline traffic so ramps and floods do not
		// inflate the quiet EWMA.
		const learnFactor = 1.5
		if a.ewmaPPS > 0 && pps > a.ewmaPPS*learnFactor {
			return
		}
		if a.ewmaBPS > 0 && bps > a.ewmaBPS*learnFactor {
			return
		}
	} else if pps > DefaultFloorPPS || bps > DefaultFloorBPS {
		// During warm-up ignore flood samples entirely so the first
		// observation cannot memorize an attack as the quiet baseline.
		return
	}
	if a.samples == 0 {
		a.ewmaPPS = pps
		a.ewmaBPS = bps
	} else {
		// Asymmetric EWMA once armed: the baseline climbs slowly under
		// sustained elevation (EWMAAlphaUp) but still falls quickly back
		// toward quiet traffic (EWMAAlpha). A constant alpha lets an
		// attacker parked at 1.49x the baseline inflate it ~10%/s and
		// reach the absolute ceiling in under a minute, silently
		// neutering the adaptive trip line.
		alphaPPS := EWMAAlpha
		alphaBPS := EWMAAlpha
		if a.ready {
			if pps > a.ewmaPPS {
				alphaPPS = EWMAAlphaUp
			}
			if bps > a.ewmaBPS {
				alphaBPS = EWMAAlphaUp
			}
		}
		a.ewmaPPS = alphaPPS*pps + (1-alphaPPS)*a.ewmaPPS
		a.ewmaBPS = alphaBPS*bps + (1-alphaBPS)*a.ewmaBPS
	}
	a.samples++
	if a.samples >= AdaptiveWarmupSamples {
		a.ready = true
	}
}

// tripLine returns the effective trip thresholds using absolute ceilings floors and adaptive headroom.
func (a *adaptiveState) tripLine(maxPPS, maxBPS, floorPPS, floorBPS float64) (ppsLimit, bpsLimit float64) {
	ppsLimit = maxPPS
	bpsLimit = maxBPS
	if !a.ready {
		return ppsLimit, bpsLimit
	}
	adaptPPS := a.ewmaPPS * AdaptiveMultiplier
	if adaptPPS < floorPPS {
		adaptPPS = floorPPS
	}
	adaptBPS := a.ewmaBPS * AdaptiveMultiplier
	if adaptBPS < floorBPS {
		adaptBPS = floorBPS
	}
	if adaptPPS < ppsLimit {
		ppsLimit = adaptPPS
	}
	if adaptBPS < bpsLimit {
		bpsLimit = adaptBPS
	}
	return ppsLimit, bpsLimit
}
