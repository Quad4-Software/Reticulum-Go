// SPDX-License-Identifier: LicenseRef-Reticulum
package codec2_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/codec2"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestCodec2RoundTrip(t *testing.T) {
	params := proto.ProfileParams(proto.ProfileBandwidthLow)
	cfg := codec2.Config{
		Bitrate:      params.Bitrate,
		Header:       0x06,
		FrameSamples: params.FrameSamples(),
		PlaySamples:  params.PlaybackFrameSamples(),
	}
	enc, err := codec2.NewEncoder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	dec, err := codec2.NewDecoder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()

	pcm := make([]int16, enc.FrameSamples())
	for i := range pcm {
		pcm[i] = int16((i % 40) * 200)
	}
	pkt, err := enc.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt) == 0 || pkt[0] != 0x06 {
		t.Fatalf("missing codec2 mode header: %x", pkt)
	}
	out, err := dec.Decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty decode")
	}
}

func TestCodec2AllModes(t *testing.T) {
	for _, br := range []int{700, 1200, 1600, 2400, 3200} {
		cfg := codec2.Config{
			Bitrate:      br,
			Header:       codec2.HeaderForBitrate(br),
			FrameSamples: codec2.SampleRate * 40 / 1000,
			PlaySamples:  proto.PlaybackSampleRate * 40 / 1000,
		}
		enc, err := codec2.NewEncoder(cfg)
		if err != nil {
			t.Fatalf("%d encode create: %v", br, err)
		}
		pcm := make([]int16, enc.FrameSamples())
		pkt, err := enc.Encode(pcm)
		_ = enc.Close()
		if err != nil {
			t.Fatalf("%d encode: %v", br, err)
		}
		if pkt[0] != codec2.HeaderForBitrate(br) {
			t.Fatalf("%d header %x", br, pkt[0])
		}
	}
}

func TestCodec2PLC(t *testing.T) {
	params := proto.ProfileParams(proto.ProfileBandwidthLow)
	dec, err := codec2.NewDecoder(codec2.Config{
		Bitrate:      params.Bitrate,
		Header:       0x06,
		FrameSamples: params.FrameSamples(),
		PlaySamples:  params.PlaybackFrameSamples(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dec.Close() }()
	out, err := dec.DecodePLC()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != params.PlaybackFrameSamples() {
		t.Fatalf("plc samples %d want %d", len(out), params.PlaybackFrameSamples())
	}
}
