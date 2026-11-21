// SPDX-License-Identifier: LicenseRef-Reticulum
package proto

import "math"

type CodecParams struct {
	Codec      byte
	SampleRate int
	Channels   int
	Bitrate    int
	FrameMs    int
	BufferN    int
	Voip       bool
}

func ProfileParams(profile int) CodecParams {
	if profile == 0 {
		profile = DefaultProfile
	}
	switch profile {
	case ProfileBandwidthUltraLow:
		return CodecParams{Codec: CodecCodec2, SampleRate: 8000, Channels: 1, Bitrate: 700, FrameMs: 400, BufferN: 2, Voip: true}
	case ProfileBandwidthVeryLow:
		return CodecParams{Codec: CodecCodec2, SampleRate: 8000, Channels: 1, Bitrate: 1600, FrameMs: 320, BufferN: 2, Voip: true}
	case ProfileBandwidthLow:
		return CodecParams{Codec: CodecCodec2, SampleRate: 8000, Channels: 1, Bitrate: 3200, FrameMs: 200, BufferN: 2, Voip: true}
	case ProfileQualityMedium:
		return opusParams(OpusVoiceMedium, 60, 5)
	case ProfileQualityHigh:
		return opusParams(OpusVoiceHigh, 60, 5)
	case ProfileQualityMax:
		return opusParams(OpusVoiceMax, 60, 5)
	case ProfileLatencyLow:
		return opusParams(OpusVoiceMedium, 20, 3)
	case ProfileLatencyUltraLow:
		return opusParams(OpusVoiceMedium, 10, 2)
	default:
		return opusParams(OpusVoiceMedium, 60, 5)
	}
}

func opusParams(opusProfile int, frameMs int, bufferN int) CodecParams {
	p := OpusProfileParams(opusProfile)
	p.FrameMs = frameMs
	p.BufferN = bufferN
	return p
}

func OpusProfileParams(opusProfile int) CodecParams {
	switch opusProfile {
	case OpusVoiceLow:
		return CodecParams{Codec: CodecOpus, SampleRate: 8000, Channels: 1, Bitrate: 6000, Voip: true}
	case OpusVoiceMedium:
		return CodecParams{Codec: CodecOpus, SampleRate: 24000, Channels: 1, Bitrate: 8000, Voip: true}
	case OpusVoiceHigh:
		return CodecParams{Codec: CodecOpus, SampleRate: 48000, Channels: 1, Bitrate: 16000, Voip: true}
	case OpusVoiceMax:
		return CodecParams{Codec: CodecOpus, SampleRate: 48000, Channels: 2, Bitrate: 32000, Voip: true}
	case OpusAudioMin:
		return CodecParams{Codec: CodecOpus, SampleRate: 8000, Channels: 1, Bitrate: 8000, Voip: false}
	case OpusAudioLow:
		return CodecParams{Codec: CodecOpus, SampleRate: 12000, Channels: 1, Bitrate: 14000, Voip: false}
	case OpusAudioMedium:
		return CodecParams{Codec: CodecOpus, SampleRate: 24000, Channels: 2, Bitrate: 28000, Voip: false}
	case OpusAudioHigh:
		return CodecParams{Codec: CodecOpus, SampleRate: 48000, Channels: 2, Bitrate: 56000, Voip: false}
	case OpusAudioMax:
		return CodecParams{Codec: CodecOpus, SampleRate: 48000, Channels: 2, Bitrate: 128000, Voip: false}
	default:
		return CodecParams{Codec: CodecOpus, SampleRate: 24000, Channels: 1, Bitrate: 8000, Voip: true}
	}
}

func (p CodecParams) FrameSamples() int {
	if p.SampleRate <= 0 || p.FrameMs <= 0 {
		return 0
	}
	return p.SampleRate * p.FrameMs / 1000
}

func (p CodecParams) PlaybackFrameSamples() int {
	if p.FrameMs <= 0 {
		return 0
	}
	return PlaybackSampleRate * p.FrameMs / 1000
}

func (p CodecParams) MaxBytesPerFrame() int {
	if p.Bitrate <= 0 || p.FrameMs <= 0 {
		return 1500
	}
	n := max(int(math.Ceil((float64(p.Bitrate)/8.0)*(float64(p.FrameMs)/1000.0))), 8)
	return n
}

func SupportsOpus(profile int) bool {
	p := ProfileParams(profile)
	return p.Codec == CodecOpus
}

func SupportsCodec2(profile int) bool {
	p := ProfileParams(profile)
	return p.Codec == CodecCodec2
}

func Codec2Mode(profile int) (bitrate int, header byte) {
	switch profile {
	case ProfileBandwidthUltraLow:
		return 700, Codec2Header700
	case ProfileBandwidthVeryLow:
		return 1600, Codec2Header1600
	case ProfileBandwidthLow:
		return 3200, Codec2Header3200
	default:
		return 0, 0
	}
}

func Codec2HeaderForBitrate(bitrate int) byte {
	switch bitrate {
	case 700:
		return Codec2Header700
	case 1200:
		return Codec2Header1200
	case 1300:
		return Codec2Header1300
	case 1400:
		return Codec2Header1400
	case 1600:
		return Codec2Header1600
	case 2400:
		return Codec2Header2400
	case 3200:
		return Codec2Header3200
	default:
		return Codec2Header3200
	}
}

func Codec2BitrateForHeader(header byte) int {
	switch header {
	case Codec2Header700:
		return 700
	case Codec2Header1200:
		return 1200
	case Codec2Header1300:
		return 1300
	case Codec2Header1400:
		return 1400
	case Codec2Header1600:
		return 1600
	case Codec2Header2400:
		return 2400
	case Codec2Header3200:
		return 3200
	default:
		return 0
	}
}

func FallbackOpusProfile(profile int) int {
	if SupportsOpus(profile) {
		return profile
	}
	return DefaultProfile
}

func AvailableProfiles() []int {
	return []int{
		ProfileBandwidthUltraLow,
		ProfileBandwidthVeryLow,
		ProfileBandwidthLow,
		ProfileQualityMedium,
		ProfileQualityHigh,
		ProfileQualityMax,
		ProfileLatencyLow,
		ProfileLatencyUltraLow,
	}
}

func NextProfile(profile int) int {
	list := AvailableProfiles()
	for i, p := range list {
		if p == profile {
			return list[(i+1)%len(list)]
		}
	}
	return 0
}
