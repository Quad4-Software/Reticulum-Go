// SPDX-License-Identifier: LicenseRef-Reticulum
package codec2

import (
	"errors"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

var ErrCodecClosed = errors.New("codec2 codec closed")

const (
	SampleRate = 8000
	Channels   = 1

	Header700  = proto.Codec2Header700
	Header1200 = proto.Codec2Header1200
	Header1300 = proto.Codec2Header1300
	Header1400 = proto.Codec2Header1400
	Header1600 = proto.Codec2Header1600
	Header2400 = proto.Codec2Header2400
	Header3200 = proto.Codec2Header3200
)

type Config struct {
	Bitrate      int
	Header       byte
	FrameSamples int
	PlaySamples  int
}

func headerForBitrate(bitrate int) byte {
	return HeaderForBitrate(bitrate)
}

func HeaderForBitrate(bitrate int) byte {
	return proto.Codec2HeaderForBitrate(bitrate)
}

func BitrateForHeader(header byte) int {
	return proto.Codec2BitrateForHeader(header)
}
