// SPDX-License-Identifier: LicenseRef-Reticulum
package filter

import "math"

const (
	defaultRate   = 48000
	twoPi         = 2 * math.Pi
	speechLowHz   = 250
	speechHighHz  = 8500
	defaultAGCdB  = -15
	pcmFullScale  = 32768
	int16Max      = 32767
	int16Min      = -32768
	agcSmoothOld  = 0.9
	agcSmoothNew  = 0.1
	agcMaxGain    = 15.85
	agcMinGain    = 0.05
	agcFloorRMS   = 1
	echoEnergyMin = 1e6
	echoNearRatio = 2
	echoAttenuate = 0.15
)

// BandPass is a biquad band-pass around the speech band.
type BandPass struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func NewBandPass(lowHz, highHz, sampleRate int) *BandPass {
	if sampleRate <= 0 {
		sampleRate = defaultRate
	}
	if lowHz <= 0 {
		lowHz = speechLowHz
	}
	if highHz <= lowHz {
		highHz = speechHighHz
	}
	f0 := float64(lowHz+highHz) / 2
	bw := float64(highHz - lowHz)
	w0 := twoPi * f0 / float64(sampleRate)
	q := f0 / bw
	alpha := math.Sin(w0) / (2 * q)
	cos := math.Cos(w0)
	b0 := alpha
	b1 := 0.0
	b2 := -alpha
	a0 := 1 + alpha
	a1 := -2 * cos
	a2 := 1 - alpha
	return &BandPass{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

func (f *BandPass) Process(pcm []int16) {
	for i, s := range pcm {
		x := float64(s)
		y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
		f.x2, f.x1 = f.x1, x
		f.y2, f.y1 = f.y1, y
		pcm[i] = clamp16(y)
	}
}

// AGC follows a target RMS in dBFS.
type AGC struct {
	target float64
	gain   float64
	paused bool
}

// NewAGC builds an AGC around a dBFS target. Upstream LXST maps dB to linear
// with 10^(db/10) (target_linear and max_gain_linear); match that convention.
func NewAGC(targetDB float64) *AGC {
	if targetDB == 0 {
		targetDB = defaultAGCdB
	}
	return &AGC{target: math.Pow(10, targetDB/10) * pcmFullScale, gain: 1}
}

func (a *AGC) Pause()  { a.paused = true }
func (a *AGC) Resume() { a.paused = false }

func (a *AGC) Process(pcm []int16) {
	if a.paused || len(pcm) == 0 {
		return
	}
	var acc float64
	for _, s := range pcm {
		v := float64(s)
		acc += v * v
	}
	rms := math.Sqrt(acc / float64(len(pcm)))
	if rms < agcFloorRMS {
		rms = agcFloorRMS
	}
	want := a.target / rms
	a.gain = a.gain*agcSmoothOld + want*agcSmoothNew
	if a.gain > agcMaxGain {
		a.gain = agcMaxGain
	}
	if a.gain < agcMinGain {
		a.gain = agcMinGain
	}
	for i, s := range pcm {
		pcm[i] = clamp16(float64(s) * a.gain)
	}
}

// EchoSuppressor attenuates capture when playback energy is high.
type EchoSuppressor struct {
	refEnergy float64
}

func NewEchoSuppressor() *EchoSuppressor {
	return &EchoSuppressor{}
}

func (e *EchoSuppressor) SetReference(pcm []int16) {
	e.refEnergy = energy(pcm)
}

func (e *EchoSuppressor) Process(pcm []int16) {
	near := energy(pcm)
	if e.refEnergy > echoEnergyMin && e.refEnergy > near*echoNearRatio {
		for i, s := range pcm {
			pcm[i] = int16(float64(s) * echoAttenuate)
		}
	}
}

func energy(pcm []int16) float64 {
	var acc float64
	for _, s := range pcm {
		v := float64(s)
		acc += v * v
	}
	return acc
}

func clamp16(v float64) int16 {
	if v > int16Max {
		return int16Max
	}
	if v < int16Min {
		return int16Min
	}
	return int16(v)
}

// ApplyGain scales samples by a decibel gain. LXST upstream converts dB to a
// linear factor with 10^(db/10) in Mixer and file sources; match that here.
func ApplyGain(pcm []int16, db float64) {
	if db == 0 || len(pcm) == 0 {
		return
	}
	g := math.Pow(10, db/10)
	for i, s := range pcm {
		pcm[i] = clamp16(float64(s) * g)
	}
}
