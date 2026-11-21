// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestCodec2ProfileParams(t *testing.T) {
	cases := []struct {
		profile int
		bitrate int
		header  byte
		frameMs int
	}{
		{proto.ProfileBandwidthUltraLow, 700, proto.Codec2Header700, 400},
		{proto.ProfileBandwidthVeryLow, 1600, proto.Codec2Header1600, 320},
		{proto.ProfileBandwidthLow, 3200, proto.Codec2Header3200, 200},
	}
	for _, tc := range cases {
		p := proto.ProfileParams(tc.profile)
		if p.Codec != proto.CodecCodec2 {
			t.Fatalf("profile %d codec %d", tc.profile, p.Codec)
		}
		if p.SampleRate != 8000 || p.Bitrate != tc.bitrate || p.FrameMs != tc.frameMs {
			t.Fatalf("profile %d params %+v", tc.profile, p)
		}
		br, hdr := proto.Codec2Mode(tc.profile)
		if br != tc.bitrate || hdr != tc.header {
			t.Fatalf("profile %d mode %d header %d", tc.profile, br, hdr)
		}
		if proto.SupportsOpus(tc.profile) || !proto.SupportsCodec2(tc.profile) {
			t.Fatalf("profile %d codec flags", tc.profile)
		}
	}
}

func TestOpusDefaultProfile(t *testing.T) {
	p := proto.ProfileParams(proto.DefaultProfile)
	if p.Codec != proto.CodecOpus || p.SampleRate != 24000 || p.Bitrate != 8000 || p.FrameMs != 60 {
		t.Fatalf("default opus params %+v", p)
	}
}

func TestProfileFromName(t *testing.T) {
	cases := map[string]int{
		"ulbw": proto.ProfileBandwidthUltraLow,
		"mq":   proto.ProfileQualityMedium,
		"ull":  proto.ProfileLatencyUltraLow,
		"":     proto.DefaultProfile,
		"nope": proto.DefaultProfile,
	}
	for name, want := range cases {
		if got := proto.ProfileFromName(name); got != want {
			t.Fatalf("ProfileFromName(%q)=%d want %d", name, got, want)
		}
	}
}

func TestNextProfileMatchesLXSTOrder(t *testing.T) {
	list := proto.AvailableProfiles()
	if len(list) != 8 {
		t.Fatalf("len %d", len(list))
	}
	if list[6] != proto.ProfileLatencyLow || list[7] != proto.ProfileLatencyUltraLow {
		t.Fatalf("ll must precede ull in cycle: %v", list)
	}
	if proto.NextProfile(proto.ProfileQualityMax) != proto.ProfileLatencyLow {
		t.Fatal("shq -> ll")
	}
	if proto.NextProfile(proto.ProfileLatencyLow) != proto.ProfileLatencyUltraLow {
		t.Fatal("ll -> ull")
	}
	if proto.NextProfile(proto.ProfileLatencyUltraLow) != proto.ProfileBandwidthUltraLow {
		t.Fatal("ull wraps to ulbw")
	}
	if proto.NextProfile(0) != 0 {
		t.Fatal("unknown profile")
	}
}
