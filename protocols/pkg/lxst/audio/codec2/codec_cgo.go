//go:build cgo && lxst_native

// SPDX-License-Identifier: LicenseRef-Reticulum
package codec2

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../third_party/codec2/include -O3
#cgo LDFLAGS: -L${SRCDIR}/../../../../third_party/codec2/lib -lcodec2
#cgo linux darwin freebsd openbsd netbsd dragonfly android windows LDFLAGS: -lm

#include <codec2/codec2.h>
#include <stdlib.h>

static int rgesp_c2_mode(int bitrate) {
	switch (bitrate) {
	case 700:
		return CODEC2_MODE_700C;
	case 1200:
		return CODEC2_MODE_1200;
	case 1300:
		return CODEC2_MODE_1300;
	case 1400:
		return CODEC2_MODE_1400;
	case 1600:
		return CODEC2_MODE_1600;
	case 2400:
		return CODEC2_MODE_2400;
	case 3200:
		return CODEC2_MODE_3200;
	default:
		return CODEC2_MODE_3200;
	}
}

static struct CODEC2 *rgesp_c2_create(int bitrate) {
	return codec2_create(rgesp_c2_mode(bitrate));
}
*/
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

// Native reports whether this build talks to libcodec2.
func Native() bool { return true }

type cgoCodec struct {
	c2      *C.struct_CODEC2
	bitrate int
	header  byte
	spf     int
	bpf     int
	frame   int
	play    int
	closed  bool
	encOut  []byte
	pcm8k   []int16
	plc     []int16
	playPCM []int16
}

func NewEncoder(cfg Config) (opus.Encoder, error) {
	return newCodec(cfg)
}

func NewDecoder(cfg Config) (opus.Decoder, error) {
	return newCodec(cfg)
}

func newCodec(cfg Config) (*cgoCodec, error) {
	if cfg.Bitrate <= 0 {
		cfg.Bitrate = 3200
	}
	if cfg.Header == 0 && cfg.Bitrate != 700 {
		cfg.Header = headerForBitrate(cfg.Bitrate)
	}
	if cfg.FrameSamples <= 0 {
		cfg.FrameSamples = SampleRate * 200 / 1000
	}
	if cfg.PlaySamples <= 0 {
		cfg.PlaySamples = proto.PlaybackSampleRate * 200 / 1000
	}
	c2 := C.rgesp_c2_create(cInt(cfg.Bitrate))
	if c2 == nil {
		return nil, fmt.Errorf("codec2_create failed for %d bps", cfg.Bitrate)
	}
	return &cgoCodec{
		c2:      c2,
		bitrate: cfg.Bitrate,
		header:  cfg.Header,
		spf:     int(C.codec2_samples_per_frame(c2)),
		bpf:     int(C.codec2_bytes_per_frame(c2)),
		frame:   cfg.FrameSamples,
		play:    cfg.PlaySamples,
	}, nil
}

func (c *cgoCodec) setMode(bitrate int, header byte) error {
	if c.c2 != nil && c.bitrate == bitrate {
		c.header = header
		return nil
	}
	next := C.rgesp_c2_create(cInt(bitrate))
	if next == nil {
		return fmt.Errorf("codec2_create failed for %d bps", bitrate)
	}
	if c.c2 != nil {
		C.codec2_destroy(c.c2)
	}
	c.c2 = next
	c.bitrate = bitrate
	c.header = header
	c.spf = int(C.codec2_samples_per_frame(next))
	c.bpf = int(C.codec2_bytes_per_frame(next))
	return nil
}

func (c *cgoCodec) Encode(pcm []int16) ([]byte, error) {
	if c.closed {
		return nil, ErrCodecClosed
	}
	if c.spf <= 0 || c.bpf <= 0 {
		return nil, fmt.Errorf("codec2 not initialised")
	}
	nFrames := len(pcm) / c.spf
	if nFrames <= 0 {
		return nil, fmt.Errorf("codec2 pcm shorter than one frame")
	}
	need := 1 + nFrames*c.bpf
	if cap(c.encOut) < need {
		c.encOut = make([]byte, need)
	}
	out := c.encOut[:need]
	out[0] = c.header
	for i := 0; i < nFrames; i++ {
		start := i * c.spf
		frame := pcm[start : start+c.spf]
		dst := out[1+i*c.bpf : 1+(i+1)*c.bpf]
		C.codec2_encode(c.c2, (*C.uchar)(unsafe.Pointer(&dst[0])), (*C.short)(unsafe.Pointer(&frame[0])))
	}
	return out, nil
}

func (c *cgoCodec) Decode(packet []byte) ([]int16, error) {
	if c.closed {
		return nil, ErrCodecClosed
	}
	if len(packet) < 1+c.bpf {
		return nil, fmt.Errorf("codec2 packet too short")
	}
	header := packet[0]
	bitrate := bitrateForHeader(header)
	if bitrate > 0 {
		if err := c.setMode(bitrate, header); err != nil {
			return nil, err
		}
	}
	body := packet[1:]
	nFrames := len(body) / c.bpf
	if nFrames <= 0 {
		return nil, fmt.Errorf("codec2 packet has no frames")
	}
	need := nFrames * c.spf
	if cap(c.pcm8k) < need {
		c.pcm8k = make([]int16, need)
	}
	pcm8k := c.pcm8k[:need]
	for i := 0; i < nFrames; i++ {
		src := body[i*c.bpf : (i+1)*c.bpf]
		dst := pcm8k[i*c.spf : (i+1)*c.spf]
		C.codec2_decode(c.c2, (*C.short)(unsafe.Pointer(&dst[0])), (*C.uchar)(unsafe.Pointer(&src[0])))
	}
	c.playPCM = opus.UpsampleInto(pcm8k, SampleRate, proto.PlaybackSampleRate, c.playPCM)
	return c.playPCM, nil
}

func (c *cgoCodec) DecodePLC() ([]int16, error) {
	if c.closed {
		return nil, ErrCodecClosed
	}
	if cap(c.plc) < c.play {
		c.plc = make([]int16, c.play)
	} else {
		c.plc = c.plc[:c.play]
		clear(c.plc)
	}
	return c.plc, nil
}

func (c *cgoCodec) SetBitrate(int) error {
	if c.closed {
		return ErrCodecClosed
	}
	return nil
}

func (c *cgoCodec) SetFEC(bool) error {
	return c.SetBitrate(0)
}

func (c *cgoCodec) SetPacketLossPerc(int) error {
	return c.SetBitrate(0)
}

func (c *cgoCodec) FrameSamples() int {
	return c.frame
}

func (c *cgoCodec) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.c2 != nil {
		C.codec2_destroy(c.c2)
		c.c2 = nil
	}
	return nil
}

func bitrateForHeader(header byte) int {
	return BitrateForHeader(header)
}

// #nosec G115 -- audio parameters are bounded before C API calls
func cInt(v int) C.int {
	return C.int(v)
}
