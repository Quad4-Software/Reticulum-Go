// SPDX-License-Identifier: LicenseRef-Reticulum
package proto_test

import (
	"testing"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestModeFromName(t *testing.T) {
	cases := map[string]int{
		"full": proto.ModeFullDuplex,
		"fdx":  proto.ModeFullDuplex,
		"half": proto.ModeHalfDuplex,
		"hdx":  proto.ModeHalfDuplex,
		"ptt":  proto.ModeHalfDuplex,
	}
	for name, want := range cases {
		if got := proto.ModeFromName(name); got != want {
			t.Fatalf("ModeFromName(%q)=%d want %d", name, got, want)
		}
	}
	if got := proto.ModeFromName("bogus"); got != proto.DefaultMode {
		t.Fatalf("default mode %d", got)
	}
}

func TestModeName(t *testing.T) {
	if proto.ModeName(proto.ModeFullDuplex) != "full" {
		t.Fatal("full name")
	}
	if proto.ModeName(proto.ModeHalfDuplex) != "half" {
		t.Fatal("half name")
	}
}

func TestProfileName(t *testing.T) {
	if proto.ProfileName(proto.ProfileQualityMedium) != "mq" {
		t.Fatal("mq name")
	}
	if proto.ProfileName(proto.ProfileFromName("ulbw")) != "ulbw" {
		t.Fatal("ulbw roundtrip")
	}
	if proto.ProfileName(0) != "unknown" {
		t.Fatal("unknown profile")
	}
}

func TestLookupRejectsUnknown(t *testing.T) {
	if _, ok := proto.LookupProfile("bogus"); ok {
		t.Fatal("profile")
	}
	if _, ok := proto.LookupMode("bogus"); ok {
		t.Fatal("mode")
	}
	if _, ok := proto.LookupProfile("ulbw"); !ok {
		t.Fatal("ulbw")
	}
	if _, ok := proto.LookupMode("ptt"); !ok {
		t.Fatal("ptt")
	}
	if proto.KnownProfile(proto.ProfileQualityMedium) != true {
		t.Fatal("mq known")
	}
	if proto.KnownProfile(0x99) {
		t.Fatal("unknown known")
	}
}
