// SPDX-License-Identifier: LicenseRef-Reticulum
package raw

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
)

const (
	Bitdepth16  byte = 0x00
	Bitdepth32  byte = 0x01
	Bitdepth64  byte = 0x02
	Bitdepth128 byte = 0x03
)

type Codec struct {
	channels int
	bitdepth byte
	frame    int
	closed   bool
	encOut   []byte
	decOut   []int16
	plc      []int16
}

func New(channels int, frameSamples int) *Codec {
	if channels < 1 {
		channels = 1
	}
	if channels > 32 {
		channels = 32
	}
	if frameSamples <= 0 {
		frameSamples = opus.DefaultFrameSize
	}
	return &Codec{channels: channels, bitdepth: Bitdepth16, frame: frameSamples}
}

func NewEncoder(channels int, frameSamples int) (opus.Encoder, error) {
	return New(channels, frameSamples), nil
}

func NewDecoder(frameSamples int) (opus.Decoder, error) {
	return New(1, frameSamples), nil
}

func (c *Codec) Encode(pcm []int16) ([]byte, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	if len(pcm) == 0 {
		return nil, fmt.Errorf("empty pcm")
	}
	width := sampleWidth(c.bitdepth)
	need := 1 + len(pcm)*c.channels*width
	if cap(c.encOut) < need {
		c.encOut = make([]byte, need)
	}
	out := c.encOut[:need]
	ch := c.channels - 1
	if ch < 0 || ch > 0x3f {
		return nil, fmt.Errorf("invalid channels")
	}
	out[0] = (c.bitdepth << 6) | byte(ch)
	off := 1
	for _, s := range pcm {
		f := float32(s) / 32768.0
		for ch := 0; ch < c.channels; ch++ {
			off += putSample(out[off:], f, c.bitdepth)
		}
	}
	return out[:off], nil
}

func (c *Codec) Decode(packet []byte) ([]int16, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	if len(packet) < 2 {
		return nil, fmt.Errorf("raw frame too short")
	}
	header := packet[0]
	channels := int(header&0x3f) + 1
	bitdepth := header >> 6
	body := packet[1:]
	width := sampleWidth(bitdepth)
	if width == 0 || len(body)%(width*channels) != 0 {
		return nil, fmt.Errorf("raw frame size mismatch")
	}
	n := len(body) / (width * channels)
	if cap(c.decOut) < n {
		c.decOut = make([]int16, n)
	}
	out := c.decOut[:n]
	for i := range n {
		base := i * width * channels
		var acc float64
		for ch := range channels {
			acc += decodeSample(body[base+ch*width:base+(ch+1)*width], bitdepth)
		}
		acc /= float64(channels)
		if acc > 1 {
			acc = 1
		}
		if acc < -1 {
			acc = -1
		}
		out[i] = int16(acc * 32767)
	}
	c.channels = channels
	c.bitdepth = bitdepth
	return out, nil
}

func (c *Codec) DecodePLC() ([]int16, error) {
	if c.closed {
		return nil, opus.ErrCodecClosed
	}
	if cap(c.plc) < c.frame {
		c.plc = make([]int16, c.frame)
	} else {
		c.plc = c.plc[:c.frame]
		clear(c.plc)
	}
	return c.plc, nil
}

func (c *Codec) SetBitrate(int) error {
	if c.closed {
		return opus.ErrCodecClosed
	}
	return nil
}

func (c *Codec) SetFEC(bool) error {
	return c.SetBitrate(0)
}

func (c *Codec) SetPacketLossPerc(int) error {
	return c.SetBitrate(0)
}

func (c *Codec) FrameSamples() int {
	return c.frame
}

func (c *Codec) Close() error {
	c.closed = true
	return nil
}

func sampleWidth(bitdepth byte) int {
	switch bitdepth {
	case Bitdepth16:
		return 2
	case Bitdepth32:
		return 4
	case Bitdepth64:
		return 8
	case Bitdepth128:
		return 16
	default:
		return 0
	}
}

func putSample(dst []byte, f float32, bitdepth byte) int {
	switch bitdepth {
	case Bitdepth16:
		binary.LittleEndian.PutUint16(dst, float32To16(f))
		return 2
	case Bitdepth32:
		binary.LittleEndian.PutUint32(dst, math.Float32bits(f))
		return 4
	case Bitdepth64:
		binary.LittleEndian.PutUint64(dst, math.Float64bits(float64(f)))
		return 8
	case Bitdepth128:
		binary.LittleEndian.PutUint64(dst, math.Float64bits(float64(f)))
		clear(dst[8:16])
		return 16
	default:
		return 0
	}
}

func decodeSample(b []byte, bitdepth byte) float64 {
	switch bitdepth {
	case Bitdepth16:
		u := binary.LittleEndian.Uint16(b)
		return float64(math.Float32frombits(float16To32(u)))
	case Bitdepth32:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case Bitdepth64:
		return math.Float64frombits(binary.LittleEndian.Uint64(b))
	case Bitdepth128:
		return math.Float64frombits(binary.LittleEndian.Uint64(b[:8]))
	default:
		return 0
	}
}

func float32To16(f float32) uint16 {
	u := math.Float32bits(f)
	sign := uint16((u >> 16) & 0x8000)
	exp := int((u>>23)&0xff) - 127 + 15
	frac := (u >> 13) & 0x3ff
	switch {
	case exp <= 0:
		return sign
	case exp >= 0x1f:
		return sign | 0x7c00
	default:
		return sign | uint16(exp)<<10 | uint16(frac) // #nosec G115 -- exp and frac fit float16 fields
	}
}

func float16To32(h uint16) uint32 {
	sign := uint32(h>>15) << 31
	exp := uint32((h >> 10) & 0x1f)
	frac := uint32(h & 0x3ff)
	switch exp {
	case 0:
		if frac == 0 {
			return sign
		}
		exp = 127 - 15 + 1
		for frac&0x400 == 0 {
			frac <<= 1
			exp--
		}
		frac &= 0x3ff
		return sign | (exp << 23) | (frac << 13)
	case 0x1f:
		if frac == 0 {
			return sign | 0x7f800000
		}
		return sign | 0x7f800000 | (frac << 13)
	default:
		return sign | ((exp + (127 - 15)) << 23) | (frac << 13)
	}
}
