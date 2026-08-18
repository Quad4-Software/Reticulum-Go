// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"testing"
	"time"
)

const (
	udpLikeBitrate   int64 = 10_000_000
	modemLikeBitrate int64 = 1200
	bulkChunk              = 500
)

func learnQuiet(e *Engine, clock *time.Time, iface string, bitrate int64, seconds int) {
	opts := AdmitOpts{Bitrate: bitrate}
	for range seconds {
		d := e.admitWithOpts(iface, 64, opts)
		if !d.Allow {
			return
		}
		advance(clock, time.Second)
	}
}

func TestOracleProtectQuietMeshDefaultFloors(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	opts := AdmitOpts{Bitrate: udpLikeBitrate}
	for range 60 {
		d := e.admitWithOpts("mesh", 180, opts)
		if !d.Allow || d.Trip {
			t.Fatalf("quiet announce cadence blocked %#v", d)
		}
		advance(clock, 800*time.Millisecond)
	}
	if e.TripCount(ReasonPPS)+e.TripCount(ReasonBPS)+e.TripCount(ReasonCoolDown) != 0 {
		t.Fatal("quiet mesh must not trip")
	}
}

func TestOracleProtectSlowRadioPhysicalRate(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	opts := AdmitOpts{Bitrate: modemLikeBitrate}
	learnQuiet(e, clock, "radio", modemLikeBitrate, 40)
	for range 30 {
		d := e.admitWithOpts("radio", 67, opts)
		if !d.Allow {
			t.Fatalf("physical-rate radio packet blocked after learn %#v", d)
		}
		advance(clock, time.Second)
	}
}

func TestOracleProtectAnnounceFloodStillSheds(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	blocked := 0
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassShedFirst}
	for range 800 {
		if d := e.admitWithOpts("udp0", 180, opts); !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("announce flood after quiet learn must shed")
	}
}

func TestOracleProtectBulkLinkAfterQuietLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep}
	blocked := 0
	for range 400 {
		d := e.admitWithOpts("udp0", bulkChunk, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, 4*time.Millisecond)
	}
	if blocked != 0 || e.InCoolDown("udp0") {
		t.Fatalf("1 Mbps class link stream after quiet learn blocked=%d ifaceCool=%v",
			blocked, e.InCoolDown("udp0"))
	}
}

func TestOracleProtectFastLinkAfterQuietLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep}
	blocked := 0
	for range 1000 {
		d := e.admitWithOpts("udp0", bulkChunk, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, time.Millisecond)
	}
	if blocked != 0 || e.InCoolDown("udp0") {
		t.Fatalf("4 Mbps class link stream after quiet learn blocked=%d ifaceCool=%v trips pps=%d bps=%d cd=%d",
			blocked, e.InCoolDown("udp0"), e.TripCount(ReasonPPS), e.TripCount(ReasonBPS), e.TripCount(ReasonCoolDown))
	}
}

func TestOracleProtectSinglePeerBulkLinkAfterQuietLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep, PeerKey: "1.2.3.4:4242"}
	blocked := 0
	for range 400 {
		d := e.admitWithOpts("udp0", bulkChunk, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, 4*time.Millisecond)
	}
	if blocked != 0 {
		t.Fatalf("single-peer 1 Mbps link stream shed by peer bucket blocked=%d", blocked)
	}
}

func TestOracleProtectNearCapacityLinkAfterQuietLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep}
	blocked := 0
	for range 800 {
		d := e.admitWithOpts("udp0", 1200, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, time.Millisecond)
	}
	if blocked != 0 || e.InCoolDown("udp0") {
		t.Fatalf("near-capacity UDP link stream after quiet learn blocked=%d ifaceCool=%v trips pps=%d bps=%d cd=%d",
			blocked, e.InCoolDown("udp0"), e.TripCount(ReasonPPS), e.TripCount(ReasonBPS), e.TripCount(ReasonCoolDown))
	}
}

func TestOracleProtectNearCapacityPeerLinkAfterQuietLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep, PeerKey: "1.2.3.4:4242"}
	blocked := 0
	for range 800 {
		d := e.admitWithOpts("udp0", 1200, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, time.Millisecond)
	}
	if blocked != 0 {
		t.Fatalf("single-peer near-capacity UDP link stream shed blocked=%d cool=%v", blocked, e.InCoolDown("udp0"))
	}
}

func TestOracleProtectAnnouncePeerFloodKeepsLinkPeer(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	flood := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassShedFirst, PeerKey: "bad:1"}
	blocked := 0
	for range 400 {
		if d := e.admitWithOpts("udp0", 180, flood); !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("announce flood from one UDP peer must still shed")
	}
	d := e.admitWithOpts("udp0", bulkChunk, AdmitOpts{
		Bitrate: udpLikeBitrate,
		Class:   ClassPreferKeep,
		PeerKey: "good:1",
	})
	if !d.Allow {
		t.Fatalf("link peer collaterally shed %#v", d)
	}
}

func TestOracleProtectAutoLearningDoesNotBlockFlood(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:                 ModeAuto,
		MaxPPS:               DefaultMaxPPS,
		FloorPPS:             DefaultFloorPPS,
		AutoLearnMinDuration: time.Hour,
		AutoLearnMinSamples:  10_000,
	})
	blocked := 0
	for range 300 {
		if d := e.AdmitPacket("new0", 64); !d.Allow {
			blocked++
		}
		advance(clock, time.Millisecond)
	}
	if e.Phase() != AutoLearning {
		t.Fatalf("phase=%s", e.Phase())
	}
	if blocked != 0 {
		t.Fatalf("auto learning blocked %d packets (pps gate should still be observe-only)", blocked)
	}
}

func TestOracleProtectTransportAutoDoesNotPromoteOnSlowRadio(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:                 ModeAuto,
		MaxPPS:               DefaultMaxPPS,
		FloorPPS:             DefaultFloorPPS,
		FloorBPS:             DefaultFloorBPS,
		TransportNode:        true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
	})
	e.NotifyInterfaces([]string{"radio"})
	opts := AdmitOpts{Bitrate: modemLikeBitrate}
	for range 80 {
		_ = e.admitWithOpts("radio", 64, opts)
		advance(clock, 2*time.Second)
	}
	if e.Phase() != AutoLearning {
		t.Fatalf("transport auto promoted on slow-radio trip line phase=%s", e.Phase())
	}
}

func TestOracleProtectHandshakeBudgetAllowsBurstOfLinks(t *testing.T) {
	e := New(Options{
		Mode:         ModePrevent,
		MaxHandshake: DefaultMaxHandshake,
		WarnWriter:   &bytes.Buffer{},
		WarnInterval: time.Hour,
	})
	var releases []func()
	t.Cleanup(func() {
		for _, r := range releases {
			r()
		}
	})
	for i := range 8 {
		d, rel := e.AdmitHandshake("tcp0")
		releases = append(releases, rel)
		if !d.Allow {
			t.Fatalf("legit concurrent handshake %d blocked %#v", i, d)
		}
	}
}

func TestOracleProtectConnBudgetAllowsNormalDials(t *testing.T) {
	e := New(Options{
		Mode:         ModePrevent,
		MaxConns:     DefaultMaxConns,
		WarnWriter:   &bytes.Buffer{},
		WarnInterval: time.Hour,
	})
	var releases []func()
	t.Cleanup(func() {
		for _, r := range releases {
			r()
		}
	})
	for i := range 16 {
		d, rel := e.AdmitConn("tcp0")
		releases = append(releases, rel)
		if !d.Allow {
			t.Fatalf("legit conn %d blocked %#v", i, d)
		}
	}
}

func TestOracleProtectMemoryShedBlocksEveryone(t *testing.T) {
	var heap uint64 = 900
	e := New(Options{
		Mode:             ModePrevent,
		SoftMemoryLimit:  1000,
		WarnWriter:       &bytes.Buffer{},
		WarnInterval:     time.Hour,
		MemorySampleFunc: func() uint64 { return heap },
	})
	e.ObserveMemory()
	d := e.admitWithOpts("udp0", 64, AdmitOpts{Class: ClassPreferKeep, Bitrate: udpLikeBitrate})
	if d.Allow {
		t.Fatal("memory shed must block prefer-keep too")
	}
}

func TestOracleProtectGigabitLinkHitsProcessCeiling(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	const gigabit int64 = 1_000_000_000
	learnQuiet(e, clock, "local0", gigabit, 40)
	opts := AdmitOpts{Bitrate: gigabit, Class: ClassPreferKeep}
	blocked := 0
	for range 1200 {
		d := e.admitWithOpts("local0", 20000, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, time.Millisecond)
	}
	if blocked == 0 {
		t.Fatal("160 Mbps class stream on a gigabit iface must still hit process maxBPS, not run unlimited")
	}
}

func TestOracleProtectPathRequestsSurviveAnnounceFlood(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	announce := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassShedFirst}
	blockedAnnounce := 0
	for range 800 {
		if d := e.admitWithOpts("udp0", 180, announce); !d.Allow {
			blockedAnnounce++
		}
	}
	if blockedAnnounce == 0 {
		t.Fatal("announce flood after quiet learn must shed")
	}
	d := e.admitWithOpts("udp0", 120, AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassUnknown})
	if !d.Allow {
		t.Fatalf("path-request class dropped during announce flood %#v", d)
	}
	d = e.admitWithOpts("udp0", bulkChunk, AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassPreferKeep})
	if !d.Allow {
		t.Fatalf("link packet dropped during announce flood %#v", d)
	}
}

func TestOracleProtectMassiveHubKeepsDiscovery(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:     ModePrevent,
		MaxPPS:   DefaultMaxPPS,
		MaxBPS:   DefaultMaxBPS,
		FloorPPS: DefaultFloorPPS,
		FloorBPS: DefaultFloorBPS,
	})
	learnQuiet(e, clock, "udp0", udpLikeBitrate, 40)
	opts := AdmitOpts{Bitrate: udpLikeBitrate, Class: ClassUnknown}
	blocked := 0
	for range 2500 {
		d := e.admitWithOpts("udp0", 120, opts)
		if !d.Allow {
			blocked++
		}
		advance(clock, 200*time.Microsecond)
	}
	if blocked != 0 {
		t.Fatalf("2500 pps path-request class after quiet learn blocked=%d (public discovery must not die)", blocked)
	}
}
