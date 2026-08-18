// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"testing"
	"time"
)

func newClockEngine(t *testing.T, opts Options) (*Engine, *time.Time) {
	t.Helper()
	clock := time.Unix(1_700_000_000, 0)
	opts.Now = func() time.Time { return clock }
	if opts.WarnWriter == nil {
		opts.WarnWriter = &bytes.Buffer{}
	}
	if opts.WarnInterval == 0 {
		opts.WarnInterval = time.Hour
	}
	e := New(opts)
	return e, &clock
}

func advance(clock *time.Time, d time.Duration) {
	*clock = clock.Add(d)
}

// TestFalsePositiveQuietAfterAdaptiveLearn ensures steady quiet traffic never trips.
func TestFalsePositiveQuietAfterAdaptiveLearn(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          2000,
		FloorPPS:        50,
		DisableCoolDown: true,
	})
	for i := range 40 {
		d := e.AdmitPacket("q0", 64)
		if !d.Allow || d.Trip {
			t.Fatalf("learn quiet trip at %d %#v", i, d)
		}
		advance(clock, time.Second)
	}
	_, _, samples, ready := e.IfaceBaseline("q0")
	if !ready || samples < AdaptiveWarmupSamples {
		t.Fatalf("baseline not ready samples=%d ready=%v", samples, ready)
	}
	tripsBefore := e.TripCount(ReasonPPS) + e.TripCount(ReasonBPS)
	for i := range 120 {
		d := e.AdmitPacket("q0", 64)
		if !d.Allow || d.Trip {
			t.Fatalf("false positive on quiet after learn at %d %#v", i, d)
		}
		advance(clock, time.Second)
	}
	tripsAfter := e.TripCount(ReasonPPS) + e.TripCount(ReasonBPS)
	if tripsAfter != tripsBefore {
		t.Fatalf("quiet traffic must not trip got delta=%d", tripsAfter-tripsBefore)
	}
}

// TestFalsePositiveBurstyLegitMeshPattern mimics announce-like bursts under the floor.
func TestFalsePositiveBurstyLegitMeshPattern(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          2000,
		FloorPPS:        80,
		DisableCoolDown: true,
	})
	// Warm baseline with sparse traffic.
	for range 30 {
		_ = e.AdmitPacket("mesh0", 128)
		advance(clock, 2*time.Second)
	}
	tripsBefore := e.TripCount(ReasonPPS)
	// Bursty legit: 8 packets in 200ms every 5s (well under floor trip line).
	for cycle := range 20 {
		for j := range 8 {
			d := e.AdmitPacket("mesh0", 200)
			if !d.Allow {
				t.Fatalf("bursty legit blocked cycle=%d j=%d %#v", cycle, j, d)
			}
			advance(clock, 25*time.Millisecond)
		}
		advance(clock, 5*time.Second)
	}
	if e.TripCount(ReasonPPS) != tripsBefore {
		t.Fatalf("bursty legit false positive trips=%d", e.TripCount(ReasonPPS)-tripsBefore)
	}
}

// TestFalseNegativeFloodStillBlockedAfterQuietBaseline is the FN twin of quiet FP.
func TestFalseNegativeFloodStillBlockedAfterQuietBaseline(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          2000,
		FloorPPS:        40,
		DisableCoolDown: true,
	})
	for range 30 {
		_ = e.AdmitPacket("fn0", 32)
		advance(clock, time.Second)
	}
	ppsLimit, _ := e.TripLine("fn0")
	if ppsLimit <= 0 {
		t.Fatal("expected trip line")
	}
	blocked := 0
	for i := 0; i < int(ppsLimit)+80; i++ {
		d := e.admitWithOpts("fn0", 32, AdmitOpts{Class: ClassShedFirst})
		if !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatalf("flood above trip line %v must block", ppsLimit)
	}
}

// TestMultiIfaceIsolationFloodOnOneQuietOnOther checks per-iface independence.
func TestMultiIfaceIsolationFloodOnOneQuietOnOther(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          30,
		FloorPPS:        10,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	for range 20 {
		d := e.AdmitPacket("quiet", 16)
		if !d.Allow || d.Trip {
			t.Fatalf("quiet iface tripped early %#v", d)
		}
		advance(clock, time.Second)
	}
	blocked := 0
	for range 80 {
		d := e.admitWithOpts("flood", 16, AdmitOpts{Class: ClassShedFirst})
		if !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("flood iface must block")
	}
	for range 20 {
		d := e.AdmitPacket("quiet", 16)
		if !d.Allow {
			t.Fatalf("quiet iface falsely blocked after peer flood %#v", d)
		}
		advance(clock, time.Second)
	}
}

// TestNearThresholdOscillationUnderLineNoTrip over-line must trip.
func TestNearThresholdOscillationUnderLineNoTrip(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          20,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	trips := uint64(0)
	for round := range 30 {
		advance(clock, time.Second)
		for i := range 18 {
			d := e.AdmitPacket("osc", 8)
			if !d.Allow {
				t.Fatalf("under-threshold false block round=%d i=%d", round, i)
			}
		}
		trips = e.TripCount(ReasonPPS)
	}
	if trips != 0 {
		t.Fatalf("under-threshold false trips=%d", trips)
	}
	advance(clock, time.Second)
	blocked := 0
	for range 40 {
		if d := e.admitWithOpts("osc", 8, AdmitOpts{Class: ClassShedFirst}); !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatal("over-threshold must block")
	}
}

// TestPoisonedWarmupFloodDoesNotArmBaseline ensures flood-only warm-up cannot ready.
func TestPoisonedWarmupFloodDoesNotArmBaseline(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          5000,
		FloorPPS:        50,
		DisableCoolDown: true,
	})
	for range 100 {
		advance(clock, time.Second)
		for range 200 {
			_ = e.AdmitPacket("poison", 64)
		}
	}
	_, _, samples, ready := e.IfaceBaseline("poison")
	if ready {
		t.Fatalf("flood-only warm-up must not mark ready samples=%d", samples)
	}
}

// TestSlowPoisonUnderFloorStillBoundedTripLine keeps absolute ceiling.
func TestSlowPoisonUnderFloorStillBoundedTripLine(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModePrevent,
		MaxPPS:          200,
		FloorPPS:        50,
		DisableCoolDown: true,
	})
	// Sustained 40pps under floor becomes the quiet baseline.
	for range 40 {
		advance(clock, time.Second)
		for range 40 {
			d := e.AdmitPacket("slow", 32)
			if !d.Allow {
				t.Fatalf("under-floor poison traffic blocked %#v", d)
			}
		}
	}
	ppsLimit, _ := e.TripLine("slow")
	if ppsLimit > 200 {
		t.Fatalf("trip line above absolute max %v", ppsLimit)
	}
	advance(clock, time.Second)
	blocked := 0
	for range 500 {
		if d := e.admitWithOpts("slow", 32, AdmitOpts{Class: ClassShedFirst}); !d.Allow {
			blocked++
		}
	}
	if blocked == 0 {
		t.Fatalf("absolute flood must still block tripLine=%v", ppsLimit)
	}
}

// TestAutoGradualRampRelearnsWithoutBlocking ensures drift demotes without FP block.
func TestAutoGradualRampRelearnsWithoutBlocking(t *testing.T) {
	var buf bytes.Buffer
	e, clock := newClockEngine(t, Options{
		Mode:                 ModeAuto,
		MaxPPS:               5000,
		FloorPPS:             30,
		WarnWriter:           &buf,
		DisableCoolDown:      true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
	})
	e.NotifyInterfaces([]string{"ramp0"})
	for range 40 {
		if d := e.AdmitPacket("ramp0", 16); !d.Allow {
			t.Fatalf("learn blocked %#v", d)
		}
		advance(clock, 2*time.Second)
	}
	if e.Phase() != AutoArmed {
		t.Fatalf("expected armed got %s", e.Phase())
	}
	ewma, _, _, _ := e.IfaceBaseline("ramp0")
	// Moderately elevated new normal under trip line.
	rate := max(int(ewma*AutoDriftFactor)+2, 4)
	if float64(rate) > ewma*AdaptiveMultiplier {
		rate = int(ewma*AutoDriftFactor) + 1
	}
	for range AutoDriftWindows + 4 {
		advance(clock, 2*time.Second)
		for j := 0; j < rate; j++ {
			d := e.AdmitPacket("ramp0", 16)
			if !d.Allow {
				t.Fatalf("ramp must not false-block during drift phase=%s %#v", e.Phase(), d)
			}
		}
		if e.Phase() == AutoLearning {
			return
		}
	}
	if e.Phase() != AutoLearning {
		t.Fatalf("expected relearn after gradual ramp got %s warn=%q", e.Phase(), buf.String())
	}
}

// TestDetectNeverFalseBlocksUnderFlood is the IDS contract under pressure.
func TestDetectNeverFalseBlocksUnderFlood(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:            ModeDetect,
		MaxPPS:          5,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	tripped := false
	for range 100 {
		d := e.AdmitPacket("ids0", 64)
		if !d.Allow {
			t.Fatalf("detect must never block %#v", d)
		}
		if d.Trip {
			tripped = true
		}
	}
	advance(clock, time.Second)
	if !tripped && e.TripCount(ReasonPPS) == 0 {
		t.Fatal("detect should still trip/count under flood")
	}
}

// TestCoolDownDoesNotFalseTripOtherIface keeps cool-down scoped.
func TestCoolDownDoesNotFalseTripOtherIface(t *testing.T) {
	e, clock := newClockEngine(t, Options{
		Mode:                ModePrevent,
		MaxPPS:              5,
		DisableAdaptive:     true,
		EnableIfaceCoolDown: true,
	})
	for range CoolDownTripThreshold + 2 {
		advance(clock, time.Second)
		for range 20 {
			_ = e.AdmitPacket("hot", 8)
		}
	}
	if !e.InCoolDown("hot") {
		t.Fatal("expected hot iface cool-down")
	}
	d := e.AdmitPacket("cold", 8)
	if !d.Allow || d.Reason == ReasonCoolDown {
		t.Fatalf("cold iface falsely cooled %#v", d)
	}
}
