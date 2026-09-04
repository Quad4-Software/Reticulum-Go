// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/health"
)

func fixedOpts(mode Mode, maxPPS float64) Options {
	return Options{
		Mode:            mode,
		MaxPPS:          maxPPS,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
	}
}

func TestParseMode(t *testing.T) {
	cases := map[string]Mode{
		"off":     ModeOff,
		"detect":  ModeDetect,
		"prevent": ModePrevent,
		"ids":     ModeDetect,
		"ips":     ModePrevent,
		"block":   ModePrevent,
		"":        ModeOff,
	}
	for in, want := range cases {
		got, ok := ParseMode(in)
		if !ok || got != want {
			t.Fatalf("ParseMode(%q) = %v %v want %v true", in, got, ok, want)
		}
	}
	if _, ok := ParseMode("bogus"); ok {
		t.Fatal("expected bogus mode to fail")
	}
}

func TestAdmitPacketOffNoTrip(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	opts := fixedOpts(ModeOff, 10)
	opts.WarnWriter = &buf
	e := New(opts)
	for range 100 {
		d := e.AdmitPacket("udp0", 100)
		if !d.Allow || d.Trip {
			t.Fatalf("off mode decision %#v", d)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warn %q", buf.String())
	}
}

func TestAdmitPacketDetectTripsAllows(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	opts := fixedOpts(ModeDetect, 5)
	opts.WarnWriter = &buf
	opts.Now = func() time.Time { return clock }
	e := New(opts)
	var tripped bool
	for range 20 {
		d := e.AdmitPacket("udp0", 10)
		if !d.Allow {
			t.Fatal("detect must allow")
		}
		if d.Trip {
			tripped = true
		}
	}
	if !tripped {
		t.Fatal("expected pps trip")
	}
	if e.TripCount(ReasonPPS) == 0 {
		t.Fatal("expected trip count")
	}
	if !strings.Contains(buf.String(), "WARNING: dos_protection detect trip reason=pps iface=udp0") {
		t.Fatalf("warn missing got %q", buf.String())
	}
}

func TestAdmitPacketPreventBlocks(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	opts := fixedOpts(ModePrevent, 5)
	opts.WarnWriter = &buf
	opts.Now = func() time.Time { return clock }
	e := New(opts)
	blocked := false
	for range 20 {
		d := e.AdmitPacket("tcp0", 10)
		if !d.Allow {
			blocked = true
			if !d.Trip || d.Reason != ReasonPPS {
				t.Fatalf("unexpected decision %#v", d)
			}
		}
	}
	if !blocked {
		t.Fatal("prevent must block after threshold")
	}
}

func TestWarnRateLimit(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	opts := fixedOpts(ModeDetect, 1)
	opts.WarnWriter = &buf
	opts.WarnInterval = time.Second
	opts.Now = func() time.Time { return clock }
	e := New(opts)
	for range 10 {
		_ = e.AdmitPacket("udp0", 1)
	}
	lines := strings.Count(buf.String(), "\n")
	if lines != 1 {
		t.Fatalf("want 1 warn line got %d %q", lines, buf.String())
	}
	clock = clock.Add(2 * time.Second)
	for range 3 {
		_ = e.AdmitPacket("udp0", 1)
	}
	if !strings.Contains(buf.String(), "suppressed=") {
		t.Fatalf("expected suppressed count %q", buf.String())
	}
}

func TestAdmitHandlerModes(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	detect := New(Options{Mode: ModeDetect, WarnWriter: &buf, WarnInterval: time.Hour, DisableCoolDown: true})
	d := detect.AdmitHandler("udp0")
	if !d.Allow || !d.Trip || d.Reason != ReasonHandler {
		t.Fatalf("detect handler %#v", d)
	}
	prevent := New(Options{Mode: ModePrevent, WarnWriter: &buf, WarnInterval: time.Hour, DisableCoolDown: true})
	d = prevent.AdmitHandler("udp0")
	if d.Allow || !d.Trip {
		t.Fatalf("prevent handler %#v", d)
	}
}

func TestAdmitConnPreventCap(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	e := New(Options{Mode: ModePrevent, MaxConns: 2, WarnWriter: &buf, WarnInterval: time.Hour, DisableCoolDown: true})
	d1, r1 := e.AdmitConn("tcp0")
	d2, r2 := e.AdmitConn("tcp0")
	d3, r3 := e.AdmitConn("tcp0")
	if !d1.Allow || !d2.Allow {
		t.Fatal("first two should allow")
	}
	if d3.Allow || !d3.Trip {
		t.Fatalf("third should block %#v", d3)
	}
	r1()
	r2()
	r3()
	d4, r4 := e.AdmitConn("tcp0")
	if !d4.Allow {
		t.Fatal("after release should allow")
	}
	r4()
}

func TestAdmitConnDetectOverCap(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	e := New(Options{Mode: ModeDetect, MaxConns: 1, WarnWriter: &buf, WarnInterval: time.Hour, DisableCoolDown: true})
	d1, r1 := e.AdmitConn("tcp0")
	d2, r2 := e.AdmitConn("tcp0")
	if !d1.Allow || !d2.Allow {
		t.Fatal("detect must allow over cap")
	}
	if !d2.Trip {
		t.Fatal("detect must trip over cap")
	}
	r1()
	r2()
}

func TestAdmitResourcePreventCap(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	e := New(Options{Mode: ModePrevent, MaxResources: 1, WarnWriter: &buf, WarnInterval: time.Hour})
	d1, r1 := e.AdmitResource(100)
	d2, r2 := e.AdmitResource(100)
	if !d1.Allow || d2.Allow {
		t.Fatalf("resource cap d1=%#v d2=%#v", d1, d2)
	}
	r1()
	r2()
}

func TestAdmitCryptoPreventCap(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	e := New(Options{Mode: ModePrevent, MaxCrypto: 1, WarnWriter: &buf, WarnInterval: time.Hour})
	d1, r1 := e.AdmitCrypto("udp0")
	d2, r2 := e.AdmitCrypto("udp0")
	if !d1.Allow || d2.Allow {
		t.Fatalf("crypto cap d1=%#v d2=%#v", d1, d2)
	}
	if e.TripCount(ReasonCrypto) == 0 {
		t.Fatal("expected crypto trip")
	}
	r1()
	r2()
}

func TestAdmitHandshakePreventCap(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	e := New(Options{Mode: ModePrevent, MaxHandshake: 1, WarnWriter: &buf, WarnInterval: time.Hour})
	d1, r1 := e.AdmitHandshake("tcp0")
	d2, r2 := e.AdmitHandshake("tcp0")
	if !d1.Allow || d2.Allow {
		t.Fatalf("handshake cap d1=%#v d2=%#v", d1, d2)
	}
	r1()
	r2()
}

func TestCoolDownAfterSustainedTrips(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:                ModePrevent,
		MaxPPS:              2,
		WarnWriter:          &buf,
		WarnInterval:        time.Hour,
		DisableAdaptive:     true,
		DisableCoolDown:     false,
		EnableIfaceCoolDown: true,
		Now:                 func() time.Time { return clock },
	})
	for range CoolDownTripThreshold + 5 {
		_ = e.AdmitPacket("cd0", 1)
		clock = clock.Add(time.Millisecond)
	}
	if e.TripCount(ReasonCoolDown) == 0 {
		t.Fatal("expected cooldown trip")
	}
	if !e.InCoolDown("cd0") {
		t.Fatal("expected iface in cooldown")
	}
	d := e.AdmitPacket("cd0", 1)
	if d.Allow || d.Reason != ReasonCoolDown {
		t.Fatalf("cooldown should block %#v", d)
	}
	clock = clock.Add(CoolDownDuration + time.Second)
	d = e.AdmitPacket("cd0", 1)
	if d.Reason == ReasonCoolDown && !d.Allow {
		t.Fatal("cooldown should have expired")
	}
}

func TestIfaceCoolDownOffByDefault(t *testing.T) {
	health.Default.Reset()
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          2,
		WarnWriter:      &bytes.Buffer{},
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		Now:             func() time.Time { return clock },
	})
	for range CoolDownTripThreshold + 20 {
		_ = e.admitWithOpts("pub0", 1, AdmitOpts{Class: ClassShedFirst})
		clock = clock.Add(time.Millisecond)
	}
	if e.InCoolDown("pub0") {
		t.Fatal("default policy must not blackhole a public iface")
	}
	clock = clock.Add(time.Second)
	d := e.admitWithOpts("pub0", 1, AdmitOpts{Class: ClassPreferKeep})
	if !d.Allow {
		t.Fatalf("quiet link packet after announce flood must still pass %#v", d)
	}
}

func TestAdaptiveRaisesTripLineAboveFloor(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          2000,
		FloorPPS:        20,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: false,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	// Quiet traffic to learn a low baseline.
	for range AdaptiveWarmupSamples + 2 {
		_ = e.AdmitPacket("ad0", 1)
		clock = clock.Add(2 * time.Second)
	}
	// Burst well above adaptive line but below absolute max.
	clock = clock.Add(time.Second)
	blocked := 0
	for range 200 {
		d := e.AdmitPacket("ad0", 1)
		if !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("adaptive prevent should block burst above quiet baseline")
	}
}

func TestObserveMemoryShed(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	var heap uint64 = 100
	e := New(Options{
		Mode:             ModePrevent,
		SoftMemoryLimit:  1000,
		WarnWriter:       &buf,
		WarnInterval:     time.Hour,
		DisableCoolDown:  true,
		MemorySampleFunc: func() uint64 { return heap },
	})
	e.ObserveMemory()
	if e.Shedding() {
		t.Fatal("should not shed yet")
	}
	heap = 900
	e.ObserveMemory()
	if !e.Shedding() {
		t.Fatal("should shed at 85%")
	}
	d := e.AdmitPacket("udp0", 1)
	if d.Allow {
		t.Fatal("prevent must block under memory shed")
	}
	heap = 600
	e.ObserveMemory()
	if e.Shedding() {
		t.Fatal("should clear shed below 70%")
	}
}

func TestMemoryShedEnforcedDuringAutoLearning(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	var heap uint64 = 100
	e := New(Options{
		Mode:             ModeAuto,
		SoftMemoryLimit:  1000,
		WarnWriter:       &buf,
		WarnInterval:     time.Hour,
		DisableCoolDown:  true,
		MemorySampleFunc: func() uint64 { return heap },
	})
	if e.Phase() != AutoLearning {
		t.Fatal("expected fresh auto engine to start in learning phase")
	}
	heap = 900
	e.ObserveMemory()
	if !e.Shedding() {
		t.Fatal("should shed at 85%")
	}
	d := e.AdmitPacket("udp0", 1)
	if d.Allow {
		t.Fatal("auto mode must enforce memory shed immediately, even while still learning a baseline")
	}
	if e.Phase() != AutoLearning {
		t.Fatal("memory enforcement should not itself promote the engine to armed")
	}
}

func TestMemoryShedStaysObserveOnlyInExplicitDetect(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	var heap uint64 = 900
	e := New(Options{
		Mode:             ModeDetect,
		SoftMemoryLimit:  1000,
		WarnWriter:       &buf,
		WarnInterval:     time.Hour,
		DisableCoolDown:  true,
		MemorySampleFunc: func() uint64 { return heap },
	})
	e.ObserveMemory()
	if !e.Shedding() {
		t.Fatal("should shed at 85%")
	}
	d := e.AdmitPacket("udp0", 1)
	if !d.Allow {
		t.Fatal("explicit detect mode must never block, even under memory shed")
	}
}

func TestPreferKeepLeniencyRecordsTrip(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          2,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	opts := AdmitOpts{Class: ClassPreferKeep}
	d1 := e.admitWithOpts("pk0", 1, opts)
	d2 := e.admitWithOpts("pk0", 1, opts)
	d3 := e.admitWithOpts("pk0", 1, opts) // pps=3 over MaxPPS(2) but within the 2x leniency band.
	if !d1.Allow || !d2.Allow || !d3.Allow {
		t.Fatalf("prefer-keep class should ride out bursts under 2x: %#v %#v %#v", d1, d2, d3)
	}
	if e.TripCount(ReasonPPS) == 0 {
		t.Fatal("prefer-keep leniency use must still be visible in trip counters, not silently invisible")
	}
	if !strings.Contains(buf.String(), "trip reason=pps") {
		t.Fatalf("expected a trip warning even while allowed under leniency, got %q", buf.String())
	}
}

func TestTripCoolDownOnlyDoesNotArmIfaceCoolDown(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:         ModePrevent,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
		Now:          func() time.Time { return clock },
	})
	for range CoolDownTripThreshold + 8 {
		d := e.tripCoolDownOnly("pk0", ReasonPPS)
		if !d.Allow {
			t.Fatalf("prefer-keep leniency must stay allowed: %#v", d)
		}
		clock = clock.Add(10 * time.Millisecond)
	}
	if e.InCoolDown("pk0") {
		t.Fatal("leniency band must not arm iface cool-down (that drops resource transfers after a quiet baseline)")
	}
	if e.TripCount(ReasonPPS) == 0 {
		t.Fatal("leniency usage should still be visible in trip counters")
	}
}

func TestPreferKeepOverTwoXCoolsDown(t *testing.T) {
	health.Default.Reset()
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          10,
		WarnWriter:      &bytes.Buffer{},
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		Now:             func() time.Time { return clock },
	})
	opts := AdmitOpts{Class: ClassPreferKeep}
	blocked := 0
	for range CoolDownTripThreshold + 20 {
		d := e.admitWithOpts("pk1", 1, opts)
		if !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("prefer-keep above 2x the trip line must shed")
	}
}

func TestPeerIsolationShedsOnlyTheFloodingPeer(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          100,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	// Peer budget is PeerBudgetFraction (0.5) of the 100 pps interface line,
	// so a single peer sending more than 50 pps should trip its own bucket
	// well before the interface aggregate itself is threatened.
	floodOpts := AdmitOpts{PeerKey: "attacker:1", Class: ClassShedFirst}
	quietOpts := AdmitOpts{PeerKey: "friend:1", Class: ClassPreferKeep}

	floodBlocked := false
	for range 80 {
		d := e.admitWithOpts("shared0", 1, floodOpts)
		if !d.Allow {
			floodBlocked = true
		}
	}
	if !floodBlocked {
		t.Fatal("expected the flooding peer to eventually be shed by its own sub-bucket")
	}

	// A different, quiet peer on the same shared interface must be
	// unaffected by the flooding peer's sub-bucket trip.
	d := e.admitWithOpts("shared0", 1, quietOpts)
	if !d.Allow {
		t.Fatalf("quiet peer should not be collaterally shed: %#v", d)
	}
}

func TestPeerIsolationDisabledFallsBackToSharedBudget(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:                 ModePrevent,
		MaxPPS:               5,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableAdaptive:      true,
		DisableCoolDown:      true,
		DisablePeerIsolation: true,
		Now:                  func() time.Time { return clock },
	})
	opts := AdmitOpts{PeerKey: "attacker:1", Class: ClassShedFirst}
	blocked := false
	for range 20 {
		d := e.admitWithOpts("shared0", 1, opts)
		if !d.Allow {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("expected the shared interface aggregate to still trip when peer isolation is disabled")
	}
}

func TestPeerSubBucketEvictionIsBounded(t *testing.T) {
	health.Default.Reset()
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          1_000_000,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	for i := range MaxTrackedPeersPerIface + 50 {
		_ = e.admitWithOpts("shared0", 1, AdmitOpts{PeerKey: fmt.Sprintf("peer-%d", i)})
	}
	e.mu.Lock()
	n := len(e.ifaces["shared0"].peers)
	e.mu.Unlock()
	if n > MaxTrackedPeersPerIface {
		t.Fatalf("peer bucket map grew unbounded: %d entries", n)
	}
}

func TestConfigureFromConfig(t *testing.T) {
	e := ConfigureFromConfig("detect", 0, "", nil)
	t.Cleanup(func() {
		e.StopMemoryMonitor()
		SetDefault(nil)
	})
	if e.Mode() != ModeDetect {
		t.Fatalf("mode %v", e.Mode())
	}
}

func TestAdmitPacketConcurrent(t *testing.T) {
	var buf bytes.Buffer
	e := New(Options{Mode: ModePrevent, MaxPPS: 1000, WarnWriter: &buf, WarnInterval: time.Hour, DisableCoolDown: true, DisableAdaptive: true})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				_ = e.AdmitPacket("udp0", 8)
			}
		})
	}
	wg.Wait()
}

func TestRateWindowBuckets(t *testing.T) {
	var w rateWindow
	now := time.Unix(1_700_000_000, 0)
	for i := range 5 {
		pps, _ := w.add(now, 10)
		if pps < float64(i+1) {
			t.Fatalf("pps=%v after %d", pps, i+1)
		}
	}
	later := now.Add(2 * time.Second)
	pps, bps := w.add(later, 100)
	if pps != 1 || bps != 100 {
		t.Fatalf("after advance pps=%v bps=%v", pps, bps)
	}
}
