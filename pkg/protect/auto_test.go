// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutoLearnsThenPromotesToPrevent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:                 ModeAuto,
		MaxPPS:               40,
		FloorPPS:             10,
		StorePath:            path,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
		Now:                  func() time.Time { return clock },
	})
	e.NotifyInterfaces([]string{"u0"})
	if e.Phase() != AutoLearning {
		t.Fatal("expected learning phase")
	}

	for i := range 40 {
		d := e.AdmitPacket("u0", 1)
		if !d.Allow {
			t.Fatalf("learning must allow quiet traffic at %d", i)
		}
		clock = clock.Add(2 * time.Second)
	}
	if e.Phase() != AutoArmed {
		t.Fatalf("expected armed after learning got %s warn=%q", e.Phase(), buf.String())
	}
	if !strings.Contains(buf.String(), "auto promote") {
		t.Fatalf("expected promote warning got %q", buf.String())
	}

	clock = clock.Add(time.Second)
	blocked := 0
	for range 80 {
		d := e.AdmitPacket("u0", 1)
		if !d.Allow {
			blocked++
		}
	}
	if e.Phase() != AutoArmed {
		t.Fatalf("flood must not demote armed phase got %s", e.Phase())
	}
	if blocked == 0 {
		t.Fatal("armed auto should block flood like prevent")
	}
}

func TestAutoPersistsAndRestoresArmed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	opts := Options{
		Mode:                 ModeAuto,
		MaxPPS:               2000,
		FloorPPS:             10,
		StorePath:            path,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
		NetworkFingerprint:   networkFingerprint([]string{"u0"}),
		Now:                  func() time.Time { return clock },
	}
	e1 := New(opts)
	for range 40 {
		_ = e1.AdmitPacket("u0", 1)
		clock = clock.Add(2 * time.Second)
	}
	if e1.Phase() != AutoArmed {
		t.Fatal("first engine should arm")
	}
	if err := e1.Persist(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("store missing: %v", err)
	}

	e2 := New(opts)
	if e2.Phase() != AutoArmed {
		t.Fatalf("restored engine should stay armed got %s", e2.Phase())
	}
	e2.mu.Lock()
	st := e2.ifaces["u0"]
	ready := st != nil && st.adapt.ready
	ewma := 0.0
	if st != nil {
		ewma = st.adapt.ewmaPPS
	}
	e2.mu.Unlock()
	if !ready || ewma <= 0 {
		t.Fatalf("restored baseline ready=%v ewma=%v", ready, ewma)
	}
}

func TestDetectPersistsBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)
	clock := time.Unix(1_700_000_000, 0)
	e1 := New(Options{
		Mode:            ModeDetect,
		StorePath:       path,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	for range 20 {
		_ = e1.AdmitPacket("d0", 8)
		clock = clock.Add(2 * time.Second)
	}
	if err := e1.Persist(); err != nil {
		t.Fatal(err)
	}
	e2 := New(Options{
		Mode:            ModeDetect,
		StorePath:       path,
		DisableCoolDown: true,
		Now:             func() time.Time { return clock },
	})
	e2.mu.Lock()
	st := e2.ifaces["d0"]
	samples := 0
	if st != nil {
		samples = st.adapt.samples
	}
	e2.mu.Unlock()
	if samples < AdaptiveWarmupSamples {
		t.Fatalf("detect should remember samples got %d", samples)
	}
}

func TestAutoRelearnOnNetworkChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, StoreFileName)
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:                 ModeAuto,
		StorePath:            path,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
		Now:                  func() time.Time { return clock },
	})
	e.NotifyInterfaces([]string{"a"})
	for range 40 {
		_ = e.AdmitPacket("a", 1)
		clock = clock.Add(2 * time.Second)
	}
	if e.Phase() != AutoArmed {
		t.Fatal("should arm first")
	}
	e.NotifyInterfaces([]string{"a", "b"})
	if e.Phase() != AutoLearning {
		t.Fatalf("network change should relearn got %s", e.Phase())
	}
	if !strings.Contains(buf.String(), "auto relearn") {
		t.Fatalf("expected relearn warn got %q", buf.String())
	}
}

func TestAutoRelearnOnDrift(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(1_700_000_000, 0)
	e := New(Options{
		Mode:                 ModeAuto,
		MaxPPS:               5000,
		FloorPPS:             20,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: time.Second,
		AutoLearnMinSamples:  10,
		Now:                  func() time.Time { return clock },
	})
	e.NotifyInterfaces([]string{"w0"})
	for range 40 {
		_ = e.AdmitPacket("w0", 1)
		clock = clock.Add(2 * time.Second)
	}
	if e.Phase() != AutoArmed {
		t.Fatal("should arm")
	}
	buf.Reset()
	// Moderately elevated rate under the trip line (new normal).
	for range AutoDriftWindows + 2 {
		clock = clock.Add(2 * time.Second)
		for range 8 {
			d := e.AdmitPacket("w0", 1)
			if !d.Allow {
				t.Fatalf("drift probe should stay under trip line got %#v", d)
			}
		}
		if e.Phase() == AutoLearning {
			if !strings.Contains(buf.String(), "auto relearn") {
				t.Fatalf("expected relearn warn got %q", buf.String())
			}
			return
		}
	}
	if e.Phase() != AutoLearning {
		t.Fatalf("drift should relearn got %s warn=%q", e.Phase(), buf.String())
	}
}

func TestParseModeAuto(t *testing.T) {
	m, ok := ParseMode("auto")
	if !ok || m != ModeAuto {
		t.Fatalf("got %v %v", m, ok)
	}
	m, ok = ParseMode("smart")
	if !ok || m != ModeAuto {
		t.Fatalf("smart got %v %v", m, ok)
	}
}
