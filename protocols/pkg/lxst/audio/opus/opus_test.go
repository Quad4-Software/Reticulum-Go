// SPDX-License-Identifier: LicenseRef-Reticulum
package opus_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

func TestEncoderDecoderRoundTrip(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, opus.DefaultChannels, 16000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()

	dec, err := opus.NewDecoder(opus.DefaultSampleRate, opus.DefaultChannels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()

	pcm := make([]int16, opus.DefaultFrameSize)
	for i := range pcm {
		pcm[i] = int16(i % 300)
	}
	pkt, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	out, err := dec.Decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty decode output")
	}
}

func TestUpsampleIntoReuses(t *testing.T) {
	in := []int16{1, 2, 3}
	buf := make([]int16, 18)
	out := opus.UpsampleInto(in, 8000, 48000, buf)
	if len(out) != 18 || &out[0] != &buf[0] {
		t.Fatal("upsample must reuse dest")
	}
}

func TestDecoderPLC(t *testing.T) {
	dec, err := opus.NewDecoder(opus.DefaultSampleRate, opus.DefaultChannels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	out, err := dec.DecodePLC()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != opus.DefaultFrameSize {
		t.Fatalf("unexpected plc frame size: %d", len(out))
	}
}

func TestLXSTDefaultProfileRoundTrip(t *testing.T) {
	params := struct {
		rate  int
		frame int
		br    int
	}{24000, 1440, 8000}
	enc, err := opus.NewEncoderConfig(opus.EncoderConfig{
		SampleRate:   params.rate,
		Channels:     1,
		Bitrate:      params.br,
		FrameSamples: params.frame,
		MaxBytes:     60,
		Voip:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	dec, err := opus.NewDecoderConfig(opus.DecoderConfig{
		SampleRate:   48000,
		Channels:     1,
		FrameSamples: 2880,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	pcm := make([]int16, params.frame)
	for i := range pcm {
		pcm[i] = int16(i % 300)
	}
	pkt, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	out, err := dec.Decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty decode output")
	}
}

func BenchmarkEncode(b *testing.B) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, opus.DefaultChannels, 16000)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	pcm := make([]int16, opus.DefaultFrameSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.Encode(pcm); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncoderFEC(t *testing.T) {
	enc, err := opus.NewEncoder(opus.DefaultSampleRate, opus.DefaultChannels, 16000)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	if err := enc.SetFEC(true); err != nil {
		t.Fatal(err)
	}
	if err := enc.SetPacketLossPerc(20); err != nil {
		t.Fatal(err)
	}
	if err := enc.SetBitrate(8000); err != nil {
		t.Fatal(err)
	}
}

func TestNativeReportsBuild(t *testing.T) {
	_ = opus.Native()
}
