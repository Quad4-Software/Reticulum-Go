// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package rate

import (
	"testing"
	"time"

	"quad4/pbt/pkg/pbt"
)

func TestNewLimiter(t *testing.T) {
	limiter := NewLimiter(10.0, 1.0)
	if limiter == nil {
		t.Fatal("NewLimiter() returned nil")
	}
}

func TestLimiter_Allow(t *testing.T) {
	limiter := NewLimiter(10.0, 1.0)

	if !limiter.Allow() {
		t.Error("Allow() should return true initially")
	}

	for range 10 {
		limiter.Allow()
	}

	if limiter.Allow() {
		t.Error("Allow() should return false after exceeding rate")
	}

	time.Sleep(1100 * time.Millisecond)

	if !limiter.Allow() {
		t.Error("Allow() should return true after waiting")
	}
}

func TestNewAnnounceRateControl(t *testing.T) {
	arc := NewAnnounceRateControl(3600.0, 3, 7200.0)
	if arc == nil {
		t.Fatal("NewAnnounceRateControl() returned nil")
	}
}

func TestAnnounceRateControl_AllowAnnounce(t *testing.T) {
	arc := NewAnnounceRateControl(1.0, 2, 2.0)

	hash := "test-dest-hash"

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true for first announce")
	}

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true for second announce (within grace)")
	}

	if arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return false for third announce (exceeds grace)")
	}

	time.Sleep(1100 * time.Millisecond)

	if !arc.AllowAnnounce(hash) {
		t.Error("AllowAnnounce() should return true after waiting")
	}
}

func TestAnnounceRateControl_AllowAnnounce_DifferentHashes(t *testing.T) {
	arc := NewAnnounceRateControl(1.0, 1, 1.0)

	hash1 := "hash1"
	hash2 := "hash2"

	if !arc.AllowAnnounce(hash1) {
		t.Error("AllowAnnounce() should return true for hash1")
	}

	if !arc.AllowAnnounce(hash2) {
		t.Error("AllowAnnounce() should return true for hash2 (different hash)")
	}
}

func TestAnnounceRateControl_DisabledByZeroTarget(t *testing.T) {
	arc := NewAnnounceRateControl(0, 0, 0)
	for i := range 100 {
		if !arc.AllowAnnounce("h") {
			t.Fatalf("AllowAnnounce should always pass when disabled (i=%d)", i)
		}
	}
}

func TestNewIngressControl(t *testing.T) {
	ic := NewIngressControl(true)
	if ic == nil {
		t.Fatal("NewIngressControl() returned nil")
	}
}

func TestIngressControl_ProcessAnnounce(t *testing.T) {
	cfg := NewIngressControlConfig()
	cfg.BurstFreq = 50.0
	cfg.BurstFreqNew = 50.0
	cfg.NewTime = 0
	cfg.BurstHold = 5 * time.Second
	cfg.BurstPenalty = 5 * time.Second
	cfg.HeldReleaseInterval = time.Second
	ic := NewIngressControlWith(cfg)

	if !ic.ProcessAnnounce("first", []byte("data"), true) {
		t.Error("first announce should pass")
	}
	for i := range 200 {
		ic.ProcessAnnounce("burst-"+itoa(i), []byte("data"), true)
	}
	if !ic.InBurst() {
		t.Fatal("expected burst-active after flood")
	}
	if ic.HeldCount() == 0 {
		t.Fatal("expected at least one held announce after burst")
	}
	if !ic.ProcessAnnounce("known", []byte("k"), false) {
		t.Error("known-destination announce must not be held mid-burst")
	}
}

func TestIngressControl_ProcessAnnounce_Disabled(t *testing.T) {
	ic := NewIngressControl(false)

	hash := "test-hash"
	data := []byte("announce data")

	if !ic.ProcessAnnounce(hash, data, true) {
		t.Error("ProcessAnnounce() should return true when disabled")
	}
	if ic.HeldCount() != 0 {
		t.Error("disabled controller must not queue anything")
	}
}

func TestIngressControl_ReleaseHeldAnnounce_RespectsTiming(t *testing.T) {
	cfg := NewIngressControlConfig()
	cfg.BurstFreq = 50.0
	cfg.BurstFreqNew = 50.0
	cfg.NewTime = 0
	cfg.BurstHold = 50 * time.Millisecond
	cfg.BurstPenalty = 50 * time.Millisecond
	cfg.HeldReleaseInterval = 10 * time.Millisecond
	ic := NewIngressControlWith(cfg)

	for i := range 200 {
		ic.ProcessAnnounce("h-"+itoa(i), []byte{byte(i)}, true)
	}
	if ic.HeldCount() == 0 {
		t.Fatal("expected announces queued during burst")
	}
	if _, _, ok := ic.ReleaseHeldAnnounce(); ok {
		t.Error("must not release while burst still active / penalty pending")
	}

	time.Sleep(150 * time.Millisecond)
	ic.ProcessAnnounce("settle", []byte{0}, false)
	time.Sleep(150 * time.Millisecond)
	ic.ProcessAnnounce("settle2", []byte{0}, false)
	if _, _, ok := ic.ReleaseHeldAnnounce(); !ok {
		t.Fatal("expected release once burst cleared and penalty elapsed")
	}
	if _, _, ok := ic.ReleaseHeldAnnounce(); ok {
		t.Error("must respect HeldReleaseInterval between releases")
	}
}

func TestIngressControl_BurstSampleMinimum(t *testing.T) {
	cfg := NewIngressControlConfig()
	cfg.BurstFreq = 0.1
	cfg.BurstFreqNew = 0.1
	cfg.NewTime = 0
	cfg.BurstHold = 5 * time.Second
	cfg.BurstPenalty = 5 * time.Second
	ic := NewIngressControlWith(cfg)

	for i := range burstSampleMinimum - 1 {
		ic.ProcessAnnounce("seed-"+itoa(i), []byte{byte(i)}, true)
	}

	if ic.InBurst() {
		t.Fatalf("burst should not engage with fewer than %d samples", burstSampleMinimum)
	}
	if ic.HeldCount() != 0 {
		t.Fatalf("no announces must be held before burst engages; held=%d", ic.HeldCount())
	}

	for i := range 50 {
		ic.ProcessAnnounce("flood-"+itoa(i), []byte{byte(i)}, true)
	}
	if !ic.InBurst() {
		t.Fatal("burst must engage once enough samples accumulate above threshold")
	}
}

func TestIngressControl_MaxHeldAnnouncesCap(t *testing.T) {
	cfg := NewIngressControlConfig()
	cfg.BurstFreq = 1.0
	cfg.BurstFreqNew = 1.0
	cfg.NewTime = 0
	cfg.MaxHeldAnnounces = 4
	cfg.BurstHold = 5 * time.Second
	cfg.BurstPenalty = 5 * time.Second
	ic := NewIngressControlWith(cfg)

	for i := range 50 {
		ic.ProcessAnnounce("h-"+itoa(i), []byte{byte(i)}, true)
	}
	if got := ic.HeldCount(); got > 4 {
		t.Fatalf("queue exceeded max: %d", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func TestPBTAnnounceRateControlDisabledAlwaysAllows(t *testing.T) {
	arc := NewAnnounceRateControl(0, 0, 0)
	prop := pbt.ForAll(
		"zero config allows every announce key",
		pbt.StringASCII(0, 64),
		func(hash string) bool {
			return arc.AllowAnnounce(hash)
		},
	)
	pbt.Check(t, prop, pbt.WithRuns(80), pbt.WithSeed(88))
}
