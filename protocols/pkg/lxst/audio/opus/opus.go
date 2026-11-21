// SPDX-License-Identifier: LicenseRef-Reticulum
package opus

import (
	"errors"
	"time"
)

var ErrCodecClosed = errors.New("opus codec closed")

const (
	DefaultSampleRate = 48000
	DefaultChannels   = 1
	DefaultFrameSize  = 960
	DefaultBitrate    = 8000
	DefaultFrameMs    = 20
	DefaultDecodeMs   = 60
	DefaultComplexity = 5
	MaxPacketSize     = 1500
	MaxLossPercent    = 100
	ApplicationVoip   = 2048
	ApplicationAudio  = 2049
)

type EncoderConfig struct {
	SampleRate   int
	Channels     int
	Bitrate      int
	FrameSamples int
	MaxBytes     int
	Voip         bool
}

type DecoderConfig struct {
	SampleRate   int
	Channels     int
	FrameSamples int
}

// Encoder encodes PCM16 frames to Opus packets.
type Encoder interface {
	Encode(pcm []int16) ([]byte, error)
	SetBitrate(bps int) error
	SetFEC(enabled bool) error
	SetPacketLossPerc(pct int) error
	FrameSamples() int
	Close() error
}

// Decoder decodes Opus packets to PCM16 frames.
type Decoder interface {
	Decode(packet []byte) ([]int16, error)
	DecodePLC() ([]int16, error)
	FrameSamples() int
	Close() error
}

func FrameDuration() time.Duration {
	return time.Duration(DefaultFrameMs) * time.Millisecond
}

func DurationOf(frameMs int) time.Duration {
	if frameMs <= 0 {
		frameMs = DefaultFrameMs
	}
	return time.Duration(frameMs) * time.Millisecond
}

func Downsample(pcm []int16, fromRate, toRate int) []int16 {
	return DownsampleInto(pcm, fromRate, toRate, nil)
}

func DownsampleInto(pcm []int16, fromRate, toRate int, buf []int16) []int16 {
	if fromRate <= 0 || toRate <= 0 || fromRate == toRate {
		return pcm
	}
	n := resampleLen(len(pcm), fromRate, toRate)
	if n <= 0 {
		return nil
	}
	if cap(buf) < n {
		buf = make([]int16, n)
	} else {
		buf = buf[:n]
	}
	if fromRate%toRate == 0 {
		ratio := fromRate / toRate
		for i := range n {
			buf[i] = pcm[i*ratio]
		}
		return buf
	}
	for i := range n {
		src := i * fromRate / toRate
		if src >= len(pcm) {
			src = len(pcm) - 1
		}
		buf[i] = pcm[src]
	}
	return buf
}

func resampleLen(n, fromRate, toRate int) int {
	if fromRate%toRate == 0 {
		return n / (fromRate / toRate)
	}
	return int(int64(n) * int64(toRate) / int64(fromRate))
}

func Upsample(pcm []int16, fromRate, toRate int) []int16 {
	return UpsampleInto(pcm, fromRate, toRate, nil)
}

func UpsampleInto(pcm []int16, fromRate, toRate int, buf []int16) []int16 {
	if fromRate <= 0 || toRate <= 0 || fromRate == toRate {
		return pcm
	}
	n := resampleLen(len(pcm), fromRate, toRate)
	if n <= 0 {
		return nil
	}
	if cap(buf) < n {
		buf = make([]int16, n)
	} else {
		buf = buf[:n]
	}
	if toRate%fromRate == 0 {
		ratio := toRate / fromRate
		for i, s := range pcm {
			base := i * ratio
			for r := range ratio {
				buf[base+r] = s
			}
		}
		return buf
	}
	for i := range n {
		src := i * fromRate / toRate
		if src >= len(pcm) {
			src = len(pcm) - 1
		}
		buf[i] = pcm[src]
	}
	return buf
}

func mixdownInterleaved(pcm []int16, channels int) []int16 {
	if channels <= 1 || len(pcm) < channels {
		return pcm
	}
	n := len(pcm) / channels
	for i := range n {
		var acc int
		base := i * channels
		for ch := range channels {
			acc += int(pcm[base+ch])
		}
		pcm[i] = int16(acc / channels) // #nosec G115 -- average of int16 samples
	}
	return pcm[:n]
}
