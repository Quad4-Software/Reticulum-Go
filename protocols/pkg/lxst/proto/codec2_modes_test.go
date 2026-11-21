// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestCodec2HeaderRoundTrip(t *testing.T) {
	for _, br := range []int{700, 1200, 1300, 1400, 1600, 2400, 3200} {
		h := proto.Codec2HeaderForBitrate(br)
		if proto.Codec2BitrateForHeader(h) != br {
			t.Fatalf("%d -> %d -> %d", br, h, proto.Codec2BitrateForHeader(h))
		}
	}
}

func TestOpusAudioProfiles(t *testing.T) {
	p := proto.OpusProfileParams(proto.OpusAudioMax)
	if p.Channels != 2 || p.Bitrate != 128000 || p.Voip {
		t.Fatalf("%+v", p)
	}
	p = proto.OpusProfileParams(proto.OpusVoiceLow)
	if p.SampleRate != 8000 || p.Bitrate != 6000 {
		t.Fatalf("%+v", p)
	}
}
