// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"math"
	"testing"
	"time"
)

// meshTraceEvent is one synthetic mesh traffic sample.
type meshTraceEvent struct {
	iface string
	bytes int
	at    time.Duration
}

// buildAnnounceLikeTrace synthesizes periodic announces plus occasional path requests.
func buildAnnounceLikeTrace(ifaces []string, duration time.Duration) []meshTraceEvent {
	var out []meshTraceEvent
	t := time.Duration(0)
	idx := 0
	for t < duration {
		iface := ifaces[idx%len(ifaces)]
		out = append(out, meshTraceEvent{iface: iface, bytes: 180, at: t})
		t += 800 * time.Millisecond
		if idx%7 == 0 {
			out = append(out, meshTraceEvent{iface: iface, bytes: 64, at: t})
			t += 50 * time.Millisecond
			out = append(out, meshTraceEvent{iface: iface, bytes: 64, at: t})
			t += 50 * time.Millisecond
		}
		idx++
	}
	return out
}

func replayTrace(e *Engine, clock *time.Time, start time.Time, events []meshTraceEvent) (allowed, blocked, trips int) {
	for _, ev := range events {
		*clock = start.Add(ev.at)
		d := e.AdmitPacket(ev.iface, ev.bytes)
		if d.Allow {
			allowed++
		} else {
			blocked++
		}
		if d.Trip {
			trips++
		}
	}
	return allowed, blocked, trips
}

// TestReplayTracePreventFalsePositiveRate measures FP rate on legit mesh-like traffic.
func TestReplayTracePreventFalsePositiveRate(t *testing.T) {
	var buf bytes.Buffer
	start := time.Unix(1_710_000_000, 0)
	clock := start
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          DefaultMaxPPS,
		FloorPPS:        DefaultFloorPPS,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	events := buildAnnounceLikeTrace([]string{"udp0", "tcp0"}, 3*time.Minute)
	allowed, blocked, trips := replayTrace(e, &clock, start, events)
	if blocked != 0 {
		t.Fatalf("false blocks on legit mesh trace blocked=%d trips=%d allowed=%d", blocked, trips, allowed)
	}
	fpRate := float64(trips) / math.Max(1, float64(allowed+blocked))
	if fpRate > 0 {
		t.Fatalf("false trip rate %v on quiet mesh trace trips=%d", fpRate, trips)
	}
}

// TestReplayTraceDetectZeroBlocks confirms IDS never sheds under the same trace.
func TestReplayTraceDetectZeroBlocks(t *testing.T) {
	start := time.Unix(1_710_000_000, 0)
	clock := start
	e := New(Options{
		Mode:            ModeDetect,
		MaxPPS:          40,
		FloorPPS:        20,
		WarnWriter:      &bytes.Buffer{},
		WarnInterval:    time.Hour,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	events := buildAnnounceLikeTrace([]string{"u0"}, 2*time.Minute)
	_, blocked, _ := replayTrace(e, &clock, start, events)
	if blocked != 0 {
		t.Fatalf("detect blocked %d on quiet trace", blocked)
	}
}

// TestReplayTraceInjectedFloodDetectedThenClears injects a flood window and checks recovery.
func TestReplayTraceInjectedFloodDetectedThenClears(t *testing.T) {
	start := time.Unix(1_710_000_000, 0)
	clock := start
	e := New(Options{
		Mode:            ModePrevent,
		MaxPPS:          80,
		FloorPPS:        20,
		WarnWriter:      &bytes.Buffer{},
		WarnInterval:    time.Hour,
		DisableCoolDown: true,
		DisableAdaptive: true,
		Now:             func() time.Time { return clock },
	})
	events := buildAnnounceLikeTrace([]string{"u0"}, time.Minute)
	_, blocked, _ := replayTrace(e, &clock, start, events)
	if blocked != 0 {
		t.Fatalf("pre-flood false blocks=%d", blocked)
	}

	floodStart := start.Add(time.Minute)
	floodBlocked := 0
	for range 200 {
		clock = floodStart
		if d := e.AdmitPacket("u0", 64); !d.Allow {
			floodBlocked++
		}
	}
	if floodBlocked == 0 {
		t.Fatal("injected flood must be blocked")
	}

	// Quiet resume in a fresh second must allow again under absolute cap.
	clock = floodStart.Add(2 * time.Second)
	d := e.AdmitPacket("u0", 64)
	if !d.Allow {
		t.Fatalf("post-flood quiet should allow %#v", d)
	}
}

// TestReplayTraceAutoLearnsThenBlocksFlood exercises auto on a recorded-style timeline.
func TestReplayTraceAutoLearnsThenBlocksFlood(t *testing.T) {
	start := time.Unix(1_710_000_000, 0)
	clock := start
	e := New(Options{
		Mode:                 ModeAuto,
		MaxPPS:               2000,
		FloorPPS:             40,
		WarnWriter:           &bytes.Buffer{},
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: 5 * time.Second,
		AutoLearnMinSamples:  12,
		Now:                  func() time.Time { return clock },
	})
	e.NotifyInterfaces([]string{"auto0"})
	// Sparse quiet trace (one packet every 2s) so stable windows can promote.
	var events []meshTraceEvent
	for i := range 50 {
		events = append(events, meshTraceEvent{
			iface: "auto0",
			bytes: 96,
			at:    time.Duration(i) * 2 * time.Second,
		})
	}
	_, blocked, _ := replayTrace(e, &clock, start, events)
	if blocked != 0 {
		t.Fatalf("auto learning false blocks=%d phase=%s", blocked, e.Phase())
	}
	if e.Phase() != AutoArmed {
		t.Fatalf("expected armed after quiet trace got %s", e.Phase())
	}
	clock = start.Add(110 * time.Second)
	floodBlocked := 0
	for range 300 {
		if d := e.AdmitPacket("auto0", 64); !d.Allow {
			floodBlocked++
		}
	}
	if floodBlocked == 0 {
		t.Fatal("armed auto must block flood after quiet trace")
	}
	if e.Phase() != AutoArmed {
		t.Fatalf("flood demoted phase=%s", e.Phase())
	}
}
