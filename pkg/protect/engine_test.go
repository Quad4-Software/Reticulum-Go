// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
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
		Mode:            ModePrevent,
		MaxPPS:          2,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: false,
		Now:             func() time.Time { return clock },
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

func TestConfigureFromConfig(t *testing.T) {
	t.Cleanup(func() { SetDefault(nil) })
	e := ConfigureFromConfig("detect", 0, "", nil)
	if e.Mode() != ModeDetect {
		t.Fatalf("mode %v", e.Mode())
	}
	e.StopMemoryMonitor()
	SetDefault(nil)
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
