// SPDX-License-Identifier: LicenseRef-Reticulum
package tone

import "math"

const twoPi = 2 * math.Pi

const (
	DialHz       = 382
	RingAHz      = 440
	RingBHz      = 480
	defaultRate  = 48000
	defaultGain  = 0.2
	int16Max     = 32767
	int16Min     = -32768
	ringOnSec    = 2
	ringCycleSec = 6
	dialCycleSec = 7
	dialOnStart  = 50
	dialOnEnd    = 2050
	busyWindowMs = 500
	busyOnAfter  = 250
)

// Source generates a sine at frequency Hz.
type Source struct {
	freq  float64
	rate  float64
	gain  float64
	phase float64
	muted bool
}

func New(freqHz, sampleRate int, gain float64) *Source {
	if sampleRate <= 0 {
		sampleRate = defaultRate
	}
	if freqHz <= 0 {
		freqHz = DialHz
	}
	if gain == 0 {
		gain = defaultGain
	}
	return &Source{freq: float64(freqHz), rate: float64(sampleRate), gain: gain}
}

func (s *Source) Mute(on bool) { s.muted = on }

func (s *Source) Fill(dst []int16) {
	if s.muted {
		clear(dst)
		return
	}
	step := twoPi * s.freq / s.rate
	amp := s.gain * int16Max
	for i := range dst {
		dst[i] = int16(amp * math.Sin(s.phase))
		s.phase += step
		if s.phase > twoPi {
			s.phase -= twoPi
		}
	}
}

// Mix adds src into dst, clipped.
func Mix(dst, src []int16) {
	n := min(len(src), len(dst))
	for i := range n {
		v := max(min(int(dst[i])+int(src[i]), int16Max), int16Min)
		dst[i] = int16(v) // #nosec G115 -- clipped to int16 range above
	}
}

// RingCadence is on for 2s and off for 4s at 48 kHz.
func RingOn(sampleIndex uint64, sampleRate int) bool {
	if sampleRate <= 0 {
		sampleRate = defaultRate
	}
	rate := uint64(sampleRate) // #nosec G115 -- sampleRate is positive after the check
	cycle := rate * ringCycleSec
	if cycle == 0 {
		return false
	}
	pos := sampleIndex % cycle
	return pos < rate*ringOnSec
}

func DialOn(sampleIndex uint64, sampleRate int) bool {
	if sampleRate <= 0 {
		sampleRate = defaultRate
	}
	rate := uint64(sampleRate) // #nosec G115 -- sampleRate is positive after the check
	cycle := rate * dialCycleSec
	if cycle == 0 {
		return false
	}
	pos := sampleIndex % cycle
	start := rate * dialOnStart / 1000
	end := rate * dialOnEnd / 1000
	return pos >= start && pos < end
}

func BusyOn(sampleIndex uint64, sampleRate int) bool {
	if sampleRate <= 0 {
		sampleRate = defaultRate
	}
	rate := uint64(sampleRate) // #nosec G115 -- sampleRate is positive after the check
	window := rate * busyWindowMs / 1000
	if window == 0 {
		return false
	}
	pos := sampleIndex % window
	return pos >= rate*busyOnAfter/1000
}
