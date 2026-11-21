// SPDX-License-Identifier: LicenseRef-Reticulum
package opus_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestAllOpusProfilesRoundTrip(t *testing.T) {
	profiles := []int{
		proto.OpusVoiceLow,
		proto.OpusVoiceMedium,
		proto.OpusVoiceHigh,
		proto.OpusVoiceMax,
		proto.OpusAudioMin,
		proto.OpusAudioLow,
		proto.OpusAudioMedium,
		proto.OpusAudioHigh,
		proto.OpusAudioMax,
	}
	for _, prof := range profiles {
		t.Run(protoOpusName(prof), func(t *testing.T) {
			p := proto.OpusProfileParams(prof)
			p.FrameMs = 60
			p.BufferN = 5
			enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
				SampleRate:   p.SampleRate,
				Channels:     p.Channels,
				Bitrate:      p.Bitrate,
				FrameSamples: p.FrameSamples(),
				MaxBytes:     p.MaxBytesPerFrame(),
				Voip:         p.Voip,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = enc.Close() }()
			dec, err := opus.NewDecoderConfig(opus.DecoderConfig{
				SampleRate:   proto.PlaybackSampleRate,
				Channels:     p.Channels,
				FrameSamples: p.PlaybackFrameSamples(),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = dec.Close() }()

			pcm := make([]int16, p.FrameSamples())
			for i := range pcm {
				pcm[i] = int16((i % 50) * 100)
			}
			pkt, err := enc.Encode(pcm)
			if err != nil {
				t.Fatal(err)
			}
			if len(pkt) == 0 {
				t.Fatal("empty opus packet")
			}
			out, err := dec.Decode(pkt)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != p.PlaybackFrameSamples() {
				t.Fatalf("decode samples %d want %d", len(out), p.PlaybackFrameSamples())
			}
		})
	}
}

func TestLXSTProfileOpusRoundTrip(t *testing.T) {
	for _, profile := range proto.AvailableProfiles() {
		if !proto.SupportsOpus(profile) {
			continue
		}
		profile := profile
		t.Run(profileName(profile), func(t *testing.T) {
			p := proto.ProfileParams(profile)
			enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
				SampleRate:   p.SampleRate,
				Channels:     p.Channels,
				Bitrate:      p.Bitrate,
				FrameSamples: p.FrameSamples(),
				MaxBytes:     p.MaxBytesPerFrame(),
				Voip:         p.Voip,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = enc.Close() }()
			dec, err := opus.NewDecoderConfig(opus.DecoderConfig{
				SampleRate:   proto.PlaybackSampleRate,
				Channels:     p.Channels,
				FrameSamples: p.PlaybackFrameSamples(),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = dec.Close() }()
			pcm := make([]int16, p.FrameSamples())
			pkt, err := enc.Encode(pcm)
			if err != nil {
				t.Fatal(err)
			}
			out, err := dec.Decode(pkt)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != p.PlaybackFrameSamples() {
				t.Fatalf("playback samples %d want %d", len(out), p.PlaybackFrameSamples())
			}
		})
	}
}

func protoOpusName(prof int) string {
	switch prof {
	case proto.OpusVoiceLow:
		return "voice_low"
	case proto.OpusVoiceMedium:
		return "voice_medium"
	case proto.OpusVoiceHigh:
		return "voice_high"
	case proto.OpusVoiceMax:
		return "voice_max"
	case proto.OpusAudioMin:
		return "audio_min"
	case proto.OpusAudioLow:
		return "audio_low"
	case proto.OpusAudioMedium:
		return "audio_medium"
	case proto.OpusAudioHigh:
		return "audio_high"
	case proto.OpusAudioMax:
		return "audio_max"
	default:
		return "unknown"
	}
}

func profileName(profile int) string {
	switch profile {
	case proto.ProfileBandwidthUltraLow:
		return "ulbw"
	case proto.ProfileBandwidthVeryLow:
		return "vlbw"
	case proto.ProfileBandwidthLow:
		return "lbw"
	case proto.ProfileQualityMedium:
		return "mq"
	case proto.ProfileQualityHigh:
		return "hq"
	case proto.ProfileQualityMax:
		return "shq"
	case proto.ProfileLatencyLow:
		return "ll"
	case proto.ProfileLatencyUltraLow:
		return "ull"
	default:
		return "unknown"
	}
}
