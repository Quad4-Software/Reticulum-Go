// SPDX-License-Identifier: LicenseRef-Reticulum
package nullc

import (
	"encoding/binary"
	"fmt"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

// Codec is a local pass-through used for ringtones and mixers.
type Codec struct {
	frame  int
	closed bool
}

func New(frameSamples int) *Codec {
	if frameSamples <= 0 {
		frameSamples = opus.DefaultFrameSize
	}
	return &Codec{frame: frameSamples}
}

func NewEncoder(frameSamples int) (opus.Encoder, error) {
	return New(frameSamples), nil
}

func NewDecoder(frameSamples int) (opus.Decoder, error) {
	return New(frameSamples), nil
}

func (c *Codec) Encode(pcm []int16) ([]byte, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty pcm")
	}
	out := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s)) // #nosec G115 -- PCM is two's complement 16-bit
	}
	return out, nil
}

func (c *Codec) Decode(packet []byte) ([]int16, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	if len(packet)%2 != 0 {
		return nil, fmt.Errorf("odd null payload")
	}
	out := make([]int16, len(packet)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(packet[i*2:])) // #nosec G115 -- PCM is two's complement 16-bit
	}
	return out, nil
}

func (c *Codec) DecodePLC() ([]int16, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	return make([]int16, c.frame), nil
}

func (c *Codec) SetBitrate(int) error {
	if c.closed {
		return opus.ErrCodecClosed
	}
	return nil
}

func (c *Codec) SetFEC(bool) error           { return c.SetBitrate(0) }
func (c *Codec) SetPacketLossPerc(int) error { return c.SetBitrate(0) }

func (c *Codec) FrameSamples() int { return c.frame }

func (c *Codec) Close() error {
	c.closed = true
	return nil
}
