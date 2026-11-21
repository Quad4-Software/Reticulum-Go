//go:build !cgo || !lxst_native

// SPDX-License-Identifier: LicenseRef-Reticulum
package codec2

import (
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

// stubStoredSamples bounds the PCM samples a stub frame carries so stub
// packets stay well under link MTU like real codec2 output does.
const stubStoredSamples = 24

// Native reports whether this build talks to libcodec2.
func Native() bool { return false }

type stubCodec struct {
	header byte
	frame  int
	play   int
	closed bool
}

func NewEncoder(cfg Config) (opus.Encoder, error) {
	return newStub(cfg), nil
}

func NewDecoder(cfg Config) (opus.Decoder, error) {
	return newStub(cfg), nil
}

func newStub(cfg Config) *stubCodec {
	if cfg.FrameSamples <= 0 {
		cfg.FrameSamples = SampleRate * 200 / 1000
	}
	if cfg.PlaySamples <= 0 {
		cfg.PlaySamples = proto.PlaybackSampleRate * 200 / 1000
	}
	if cfg.Header == 0 && cfg.Bitrate != 700 {
		cfg.Header = headerForBitrate(cfg.Bitrate)
	}
	return &stubCodec{header: cfg.Header, frame: cfg.FrameSamples, play: cfg.PlaySamples}
}

func (s *stubCodec) Encode(pcm []int16) ([]byte, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty pcm")
	}
	n := len(pcm)
	k := n
	if k > stubStoredSamples {
		k = stubStoredSamples
	}
	out := make([]byte, 7+k*2)
	out[0] = s.header
	out[1] = 'S'
	out[2] = 'T'
	out[3] = byte(n >> 8)
	out[4] = byte(n)
	out[5] = byte(k >> 8)
	out[6] = byte(k)
	for i := 0; i < k; i++ {
		v := pcm[i*n/k]
		out[7+i*2] = byte(v >> 8)
		out[7+i*2+1] = byte(v)
	}
	return out, nil
}

func (s *stubCodec) Decode(packet []byte) ([]int16, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	if len(packet) < 7 || packet[1] != 'S' || packet[2] != 'T' {
		return nil, fmt.Errorf("invalid stub codec2 packet")
	}
	n := int(packet[3])<<8 | int(packet[4])
	k := int(packet[5])<<8 | int(packet[6])
	if n <= 0 || k <= 0 || len(packet) < 7+k*2 {
		return nil, fmt.Errorf("truncated stub codec2 packet")
	}
	pcm := make([]int16, n)
	for i := range pcm {
		pcm[i] = int16(packet[7+(i*k/n)*2])<<8 | int16(packet[7+(i*k/n)*2+1])
	}
	return opus.Upsample(pcm, SampleRate, proto.PlaybackSampleRate), nil
}

func (s *stubCodec) DecodePLC() ([]int16, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	return make([]int16, s.play), nil
}

func (s *stubCodec) SetBitrate(int) error {
	if s.closed {
		return ErrCodecClosed
	}
	return nil
}

func (s *stubCodec) SetFEC(bool) error {
	return s.SetBitrate(0)
}

func (s *stubCodec) SetPacketLossPerc(int) error {
	return s.SetBitrate(0)
}

func (s *stubCodec) FrameSamples() int {
	return s.frame
}

func (s *stubCodec) Close() error {
	s.closed = true
	return nil
}
