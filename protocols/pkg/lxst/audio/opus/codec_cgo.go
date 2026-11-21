//go:build cgo && lxst_native

// SPDX-License-Identifier: LicenseRef-Reticulum
package opus

/*
#cgo CFLAGS: -I${SRCDIR}/../../../../third_party/opus/include -O3
#cgo LDFLAGS: -L${SRCDIR}/../../../../third_party/opus/lib -lopus
#cgo linux darwin freebsd openbsd netbsd dragonfly android windows LDFLAGS: -lm

#include <opus/opus.h>
#include <opus/opus_defines.h>
#include <stdlib.h>

static int rgesp_opus_set_bitrate(OpusEncoder *enc, int bitrate) {
	return opus_encoder_ctl(enc, OPUS_SET_BITRATE(bitrate));
}

static int rgesp_opus_set_complexity(OpusEncoder *enc, int complexity) {
	return opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(complexity));
}

static int rgesp_opus_set_fec(OpusEncoder *enc, int enabled) {
	return opus_encoder_ctl(enc, OPUS_SET_INBAND_FEC(enabled));
}

static int rgesp_opus_set_pl(OpusEncoder *enc, int pct) {
	return opus_encoder_ctl(enc, OPUS_SET_PACKET_LOSS_PERC(pct));
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Native reports whether this build talks to libopus.
func Native() bool { return true }

type cgoEncoder struct {
	enc      *C.OpusEncoder
	frame    int
	maxBytes int
	closed   bool
	out      []byte
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
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = DefaultSampleRate
	}
	if cfg.Channels <= 0 {
		cfg.Channels = DefaultChannels
	}
	if cfg.Bitrate <= 0 {
		cfg.Bitrate = DefaultBitrate
	}
	if cfg.FrameSamples <= 0 {
		cfg.FrameSamples = cfg.SampleRate * DefaultFrameMs / 1000
	}
	var app C.int = C.OPUS_APPLICATION_VOIP
	if !cfg.Voip {
		app = C.OPUS_APPLICATION_AUDIO
	}
	var err C.int
	enc := C.opus_encoder_create(cInt(cfg.SampleRate), cInt(cfg.Channels), app, &err)
	if err != C.OPUS_OK {
		return nil, fmt.Errorf("opus_encoder_create: %d", int(err))
	}
	if cErr := C.rgesp_opus_set_bitrate(enc, cInt(cfg.Bitrate)); cErr != C.OPUS_OK {
		C.opus_encoder_destroy(enc)
		return nil, fmt.Errorf("opus_set_bitrate: %d", int(cErr))
	}
	if cErr := C.rgesp_opus_set_complexity(enc, cInt(DefaultComplexity)); cErr != C.OPUS_OK {
		C.opus_encoder_destroy(enc)
		return nil, fmt.Errorf("opus_set_complexity: %d", int(cErr))
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = MaxPacketSize
	}
	return &cgoEncoder{enc: enc, frame: cfg.FrameSamples, maxBytes: maxBytes, out: make([]byte, maxBytes)}, nil
}

func (e *cgoEncoder) Encode(pcm []int16) ([]byte, error) {
	if e.closed {
		return nil, ErrCodecClosed
	}
	if len(pcm) < e.frame {
		return nil, fmt.Errorf("pcm frame too short: got %d want %d", len(pcm), e.frame)
	}
	if cap(e.out) < e.maxBytes {
		e.out = make([]byte, e.maxBytes)
	}
	e.out = e.out[:e.maxBytes]
	n := C.opus_encode(
		e.enc,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		cInt(e.frame),
		(*C.uchar)(unsafe.Pointer(&e.out[0])),
		cInt(len(e.out)),
	)
	if n < 0 {
		return nil, fmt.Errorf("opus_encode: %d", int(n))
	}
	return e.out[:n], nil
}

func (e *cgoEncoder) SetBitrate(bps int) error {
	if e.closed {
		return ErrCodecClosed
	}
	if cErr := C.rgesp_opus_set_bitrate(e.enc, cInt(bps)); cErr != C.OPUS_OK {
		return fmt.Errorf("opus_set_bitrate: %d", int(cErr))
	}
	return nil
}

func (e *cgoEncoder) SetFEC(enabled bool) error {
	if e.closed {
		return ErrCodecClosed
	}
	v := 0
	if enabled {
		v = 1
	}
	if cErr := C.rgesp_opus_set_fec(e.enc, cInt(v)); cErr != C.OPUS_OK {
		return fmt.Errorf("opus_set_fec: %d", int(cErr))
	}
	return nil
}

func (e *cgoEncoder) SetPacketLossPerc(pct int) error {
	if e.closed {
		return ErrCodecClosed
	}
	if pct < 0 {
		pct = 0
	}
	if pct > MaxLossPercent {
		pct = MaxLossPercent
	}
	if cErr := C.rgesp_opus_set_pl(e.enc, cInt(pct)); cErr != C.OPUS_OK {
		return fmt.Errorf("opus_set_packet_loss: %d", int(cErr))
	}
	return nil
}

func (e *cgoEncoder) FrameSamples() int {
	return e.frame
}

func (e *cgoEncoder) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	C.opus_encoder_destroy(e.enc)
	e.enc = nil
	return nil
}

type cgoDecoder struct {
	dec      *C.OpusDecoder
	frame    int
	channels int
	closed   bool
	pcm      []int16
}

func NewDecoder(sampleRate, channels int) (Decoder, error) {
	return NewDecoderConfig(DecoderConfig{
		SampleRate:   sampleRate,
		Channels:     channels,
		FrameSamples: DefaultFrameSize,
	})
}

func NewDecoderConfig(cfg DecoderConfig) (Decoder, error) {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = DefaultSampleRate
	}
	if cfg.Channels <= 0 {
		cfg.Channels = DefaultChannels
	}
	if cfg.FrameSamples <= 0 {
		cfg.FrameSamples = cfg.SampleRate * DefaultDecodeMs / 1000
	}
	var err C.int
	dec := C.opus_decoder_create(cInt(cfg.SampleRate), cInt(cfg.Channels), &err)
	if err != C.OPUS_OK {
		return nil, fmt.Errorf("opus_decoder_create: %d", int(err))
	}
	return &cgoDecoder{
		dec:      dec,
		frame:    cfg.FrameSamples,
		channels: cfg.Channels,
		pcm:      make([]int16, cfg.FrameSamples*cfg.Channels),
	}, nil
}

func (d *cgoDecoder) Decode(packet []byte) ([]int16, error) {
	if d.closed {
		return nil, ErrCodecClosed
	}
	if len(packet) == 0 {
		return nil, fmt.Errorf("empty opus packet")
	}
	need := d.frame * d.channels
	if need < d.frame {
		need = d.frame
	}
	if cap(d.pcm) < need {
		d.pcm = make([]int16, need)
	}
	d.pcm = d.pcm[:need]
	n := C.opus_decode(
		d.dec,
		(*C.uchar)(unsafe.Pointer(&packet[0])),
		cInt(len(packet)),
		(*C.opus_int16)(unsafe.Pointer(&d.pcm[0])),
		cInt(d.frame),
		0,
	)
	if n < 0 {
		return nil, fmt.Errorf("opus_decode: %d", int(n))
	}
	return mixdownInterleaved(d.pcm[:int(n)*d.channels], d.channels), nil
}

func (d *cgoDecoder) DecodePLC() ([]int16, error) {
	if d.closed {
		return nil, ErrCodecClosed
	}
	need := d.frame * d.channels
	if need < d.frame {
		need = d.frame
	}
	if cap(d.pcm) < need {
		d.pcm = make([]int16, need)
	}
	d.pcm = d.pcm[:need]
	n := C.opus_decode(
		d.dec,
		nil,
		0,
		(*C.opus_int16)(unsafe.Pointer(&d.pcm[0])),
		cInt(d.frame),
		0,
	)
	if n < 0 {
		return nil, fmt.Errorf("opus_decode_plc: %d", int(n))
	}
	return mixdownInterleaved(d.pcm[:int(n)*d.channels], d.channels), nil
}

// #nosec G115 -- audio parameters are bounded before C API calls
func cInt(v int) C.int {
	return C.int(v)
}

func (d *cgoDecoder) FrameSamples() int {
	return d.frame
}

func (d *cgoDecoder) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	C.opus_decoder_destroy(d.dec)
	d.dec = nil
	return nil
}
