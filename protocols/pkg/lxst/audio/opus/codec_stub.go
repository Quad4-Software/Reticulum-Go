//go:build !cgo || !lxst_native

// SPDX-License-Identifier: LicenseRef-Reticulum
package opus

import "fmt"

// stubStoredSamples bounds the PCM samples a stub frame carries so stub
// packets stay well under link MTU like real codec output does.
const stubStoredSamples = 24

// Native reports whether this build talks to libopus.
func Native() bool { return false }

type stubEncoder struct {
	bitrate int
	frame   int
	closed  bool
}

func NewEncoder(sampleRate, channels, bitrate int) (Encoder, error) {
	return NewEncoderConfig(EncoderConfig{
		SampleRate:   sampleRate,
		Channels:     channels,
		Bitrate:      bitrate,
		FrameSamples: DefaultFrameSize,
		Voip:         true,
	})
}

func NewEncoderConfig(cfg EncoderConfig) (Encoder, error) {
	if cfg.Bitrate <= 0 {
		cfg.Bitrate = DefaultBitrate
	}
	if cfg.FrameSamples <= 0 {
		cfg.FrameSamples = DefaultFrameSize
	}
	return &stubEncoder{bitrate: cfg.Bitrate, frame: cfg.FrameSamples}, nil
}

func (s *stubEncoder) Encode(pcm []int16) ([]byte, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty pcm")
	}
	if len(pcm) < s.frame {
		return nil, fmt.Errorf("short pcm")
	}
	n := s.frame
	k := n
	if k > stubStoredSamples {
		k = stubStoredSamples
	}
	out := make([]byte, 6+k*2)
	out[0] = 'S'
	out[1] = 'T'
	out[2] = byte(n >> 8)
	out[3] = byte(n)
	out[4] = byte(k >> 8)
	out[5] = byte(k)
	for i := 0; i < k; i++ {
		v := pcm[i*n/k]
		out[6+i*2] = byte(v >> 8)
		out[6+i*2+1] = byte(v)
	}
	return out, nil
}

func (s *stubEncoder) SetBitrate(bps int) error {
	if s.closed {
		return ErrCodecClosed
	}
	s.bitrate = bps
	return nil
}

func (s *stubEncoder) SetFEC(bool) error {
	return s.SetBitrate(s.bitrate)
}

func (s *stubEncoder) SetPacketLossPerc(int) error {
	return s.SetBitrate(s.bitrate)
}

func (s *stubEncoder) FrameSamples() int {
	return s.frame
}

func (s *stubEncoder) Close() error {
	s.closed = true
	return nil
}

type stubDecoder struct {
	frame  int
	closed bool
}

func NewDecoder(sampleRate, channels int) (Decoder, error) {
	return NewDecoderConfig(DecoderConfig{
		SampleRate:   sampleRate,
		Channels:     channels,
		FrameSamples: DefaultFrameSize,
	})
}

func NewDecoderConfig(cfg DecoderConfig) (Decoder, error) {
	frame := cfg.FrameSamples
	if frame <= 0 {
		frame = DefaultFrameSize
	}
	return &stubDecoder{frame: frame}, nil
}

func (s *stubDecoder) Decode(packet []byte) ([]int16, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	if len(packet) < 6 || packet[0] != 'S' || packet[1] != 'T' {
		return nil, fmt.Errorf("invalid stub packet")
	}
	k := int(packet[4])<<8 | int(packet[5])
	if k <= 0 || len(packet) < 6+k*2 {
		return nil, fmt.Errorf("truncated stub packet")
	}
	// Real opus decodes to the decoder's configured frame length at the
	// playback rate; mirror that contract so callers see a fixed frame size.
	out := make([]int16, s.frame)
	for i := range out {
		v := int16(packet[6+(i*k/s.frame)*2])<<8 | int16(packet[6+(i*k/s.frame)*2+1])
		out[i] = v
	}
	return out, nil
}

func (s *stubDecoder) DecodePLC() ([]int16, error) {
	if s.closed {
		return nil, ErrCodecClosed
	}
	out := make([]int16, s.frame)
	return out, nil
}

func (s *stubDecoder) FrameSamples() int {
	return s.frame
}

func (s *stubDecoder) Close() error {
	s.closed = true
	return nil
}
