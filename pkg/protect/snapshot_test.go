// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package protect

import (
	"testing"
	"time"
)

func TestSnapshotAutoLearning(t *testing.T) {
	e := New(Options{
		Mode:         ModeAuto,
		WarnInterval: time.Hour,
	})
	e.NotifyInterfaces([]string{"u0"})
	snap := e.snapshot()
	if snap.Mode != "auto" {
		t.Fatalf("mode=%q", snap.Mode)
	}
	if snap.Phase != "learning" {
		t.Fatalf("phase=%q", snap.Phase)
	}
	if snap.Enforcement != "detect" {
		t.Fatalf("enforcement=%q want detect while learning", snap.Enforcement)
	}
}

func TestSnapshotTripCounts(t *testing.T) {
	e := New(Options{Mode: ModeDetect, WarnInterval: time.Hour})
	_ = e.AdmitHandler("h0")
	snap := e.snapshot()
	if snap.TripCounts.Handler != 1 {
		t.Fatalf("handler trips=%d", snap.TripCounts.Handler)
	}
}

func TestActivePressureArmed(t *testing.T) {
	e := New(Options{Mode: ModePrevent, WarnInterval: time.Hour})
	_ = e.AdmitHandler("x0")
	snap := e.snapshot()
	if !snap.ActivePressure() {
		t.Fatal("expected active pressure when prevent armed and tripped")
	}
}
