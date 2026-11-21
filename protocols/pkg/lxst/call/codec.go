// SPDX-License-Identifier: LicenseRef-Reticulum
package call

import (
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/codec2"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/nullc"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/opus"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/audio/raw"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func newProfileEncoder(params proto.CodecParams) (opus.Encoder, error) {
	switch params.Codec {
	case proto.CodecCodec2:
		return codec2.NewEncoder(codec2.Config{
			Bitrate:      params.Bitrate,
			Header:       proto.Codec2HeaderForBitrate(params.Bitrate),
			FrameSamples: params.FrameSamples(),
			PlaySamples:  params.PlaybackFrameSamples(),
		})
	case proto.CodecRaw:
		return raw.NewEncoder(params.Channels, params.FrameSamples())
	case proto.CodecNull:
		return nullc.NewEncoder(params.FrameSamples())
	default:
		return opus.NewEncoderConfig(opus.EncoderConfig{
			SampleRate:   params.SampleRate,
			Channels:     params.Channels,
			Bitrate:      params.Bitrate,
			FrameSamples: params.FrameSamples(),
			MaxBytes:     params.MaxBytesPerFrame(),
			Voip:         params.Voip,
		})
	}
}

func newProfileDecoder(params proto.CodecParams) (opus.Decoder, error) {
	return newDecoderForCodec(params.Codec, params)
}

func newDecoderForCodec(codec byte, params proto.CodecParams) (opus.Decoder, error) {
	switch codec {
	case proto.CodecCodec2:
		return codec2.NewDecoder(codec2.Config{
			Bitrate:      params.Bitrate,
			Header:       proto.Codec2HeaderForBitrate(params.Bitrate),
			FrameSamples: params.FrameSamples(),
			PlaySamples:  params.PlaybackFrameSamples(),
		})
	case proto.CodecRaw:
		return raw.NewDecoder(params.PlaybackFrameSamples())
	case proto.CodecNull:
		return nullc.NewDecoder(params.PlaybackFrameSamples())
	default:
		ch := params.Channels
		if ch <= 0 {
			ch = proto.PlaybackChannels
		}
		return opus.NewDecoderConfig(opus.DecoderConfig{
			SampleRate:   proto.PlaybackSampleRate,
			Channels:     ch,
			FrameSamples: params.PlaybackFrameSamples(),
		})
	}
}

func (c *Call) ensureRecvDecoder(codec byte, payload []byte) error {
	c.mutex.Lock()
	if c.decoder != nil && c.recvKind == codec {
		c.mutex.Unlock()
		return nil
	}
	if c.decoder != nil && c.recvKind != codec {
		if codec != proto.CodecCodec2 || c.recvKind != proto.CodecCodec2 {
			c.mutex.Unlock()
			return nil
		}
	}
	params := c.params
	c.mutex.Unlock()

	if codec == proto.CodecCodec2 && len(payload) > 0 {
		if br := proto.Codec2BitrateForHeader(payload[0]); br > 0 {
			params.Bitrate = br
			params.Codec = proto.CodecCodec2
		}
	} else {
		params.Codec = codec
	}
	dec, err := newDecoderForCodec(codec, params)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	if c.decoder != nil && c.recvKind == codec {
		c.mutex.Unlock()
		_ = dec.Close()
		return nil
	}
	old := c.decoder
	c.decoder = dec
	c.recvKind = codec
	c.recvCodec.Store(uint32(codec))
	c.mutex.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}
