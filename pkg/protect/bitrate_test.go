// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
)

func TestScaledFloorsHighBitrate(t *testing.T) {
	pps, bps := scaledFloors(10_000_000, DefaultFloorPPS, DefaultFloorBPS, DefaultMaxPPS, DefaultMaxBPS)
	if pps <= DefaultFloorPPS {
		t.Fatalf("pps=%v want above default floor", pps)
	}
	if bps <= DefaultFloorBPS {
		t.Fatalf("bps=%v want above default floor", bps)
	}
}

func TestPeekPacketClass(t *testing.T) {
	announce := byte(0x01)
	linkPkt := byte(wirePacketLink)
	linkData := byte(wireDestTypeLink << wireDestTypeShift)
	if PeekPacketClass([]byte{announce, 0}) != ClassShedFirst {
		t.Fatal("announce")
	}
	if PeekPacketClass([]byte{linkPkt, 0}) != ClassPreferKeep {
		t.Fatal("link packet type")
	}
	if PeekPacketClass([]byte{linkData, 0}) != ClassPreferKeep {
		t.Fatal("link dest type")
	}
}

func TestPriorityShedPreferKeep(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          100,
		FloorPPS:        10,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	for range 20 {
		_ = e.admitWithOpts("p0", 64, AdmitOpts{Class: ClassShedFirst})
	}
	allowed := 0
	for range 30 {
		d := e.admitWithOpts("p0", 64, AdmitOpts{Class: ClassPreferKeep})
		if d.Allow {
			allowed++
		}
	}
	if allowed == 0 {
		t.Fatal("expected some prefer-keep packets under 2x line")
	}
}

func TestConfigureFromConfigTransportLearnDuration(t *testing.T) {
	e := ConfigureFromConfig("auto", 0, "", &common.ReticulumConfig{EnableTransport: true})
	if e.autoLearnMinDuration < AutoLearnMinDuration*2 {
		t.Fatalf("duration=%v", e.autoLearnMinDuration)
	}
}
