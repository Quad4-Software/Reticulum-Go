// SPDX-License-Identifier: LicenseRef-Reticulum
package media

import (
	"math"
	"sync"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

// LinkMetrics carries path telemetry used for adaptation.
type LinkMetrics struct {
	RTT      float64
	RSSI     float64
	SNR      float64
	Q        float64
	LossRate float64
	JitterMs float64
	Bitrate  int
	UseFEC   bool
}

// AdaptiveController tunes bitrate, FEC, and jitter depth from link metrics.
type AdaptiveController struct {
	mutex sync.Mutex

	bitrate    int
	useFEC     bool
	jitterMs   int
	lastUpdate time.Time
}

const (
	defaultBitrate  = 16000
	defaultJitterMs = 80
	poorScore       = 0.35
	fairScore       = 0.65
	bitrateStepDown = 4000
	bitrateStepUp   = 2000
	jitterStepPoor  = 20
	jitterStepFair  = 5
	jitterStepGood  = 10
	jitterFloor     = 40
	jitterCeilPoor  = 300
	jitterCeilFair  = 200
	jitterCeilGood  = 160
	lossFECOn       = 0.05
	lossFECOff      = 0.01
	rttNorm         = 2.0
	jitterNorm      = 200.0
	snrNorm         = 30.0
	wRTT            = 0.35
	wLoss           = 0.35
	wJitter         = 0.15
	wRF             = 0.15
)

func NewAdaptiveController() *AdaptiveController {
	return &AdaptiveController{
		bitrate:  defaultBitrate,
		jitterMs: defaultJitterMs,
	}
}

func (a *AdaptiveController) Update(m LinkMetrics) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	score := scoreMetrics(m)
	switch {
	case score < poorScore:
		a.bitrate = clampInt(a.bitrate-bitrateStepDown, proto.MinBitrate, proto.MaxBitrate)
		a.useFEC = true
		a.jitterMs = clampInt(a.jitterMs+jitterStepPoor, jitterFloor, jitterCeilPoor)
	case score < fairScore:
		a.useFEC = m.LossRate > lossFECOn
		a.jitterMs = clampInt(a.jitterMs+jitterStepFair, jitterFloor, jitterCeilFair)
	default:
		a.bitrate = clampInt(a.bitrate+bitrateStepUp, proto.MinBitrate, proto.MaxBitrate)
		if m.LossRate < lossFECOff {
			a.useFEC = false
		}
		a.jitterMs = clampInt(a.jitterMs-jitterStepGood, jitterFloor, jitterCeilGood)
	}
	a.lastUpdate = time.Now()
}

func (a *AdaptiveController) Bitrate() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.bitrate
}

func (a *AdaptiveController) UseFEC() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.useFEC
}

func (a *AdaptiveController) JitterMs() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.jitterMs
}

func scoreMetrics(m LinkMetrics) float64 {
	rttScore := 1.0 - clampFloat(m.RTT/rttNorm, 0, 1)
	lossScore := 1.0 - clampFloat(m.LossRate, 0, 1)
	jitterScore := 1.0 - clampFloat(m.JitterMs/jitterNorm, 0, 1)
	rfScore := clampFloat((m.Q+m.SNR/snrNorm)/2.0, 0, 1)
	return clampFloat((rttScore*wRTT)+(lossScore*wLoss)+(jitterScore*wJitter)+(rfScore*wRF), 0, 1)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
