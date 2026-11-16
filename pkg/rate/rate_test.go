// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package rate

import (
	"testing"
	"time"

	"github.com/Quad4-Software/pbt/pkg/pbt"
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

// Regression: grace=0 with an unseen destination must not panic on the
// empty history slice.
func TestAnnounceRateControl_ZeroGraceEmptyHistory(t *testing.T) {
	arc := NewAnnounceRateControl(60.0, 0, 0)
	if !arc.AllowAnnounce("fresh") {
		t.Error("first announce for a new destination should pass with grace=0")
	}
	if arc.AllowAnnounce("fresh") {
		t.Error("second announce inside the target window should be denied with grace=0")
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
		t.Fatalf("no announces must be held before burst engages. Held=%d", ic.HeldCount())
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

// newBurstingIngress returns an IngressControl driven into a burst with a
// fast config so hold/release tests avoid long sleeps.
func newBurstingIngress(t *testing.T) *IngressControl {
	t.Helper()
	cfg := NewIngressControlConfig()
	cfg.BurstFreq = 50.0
	cfg.BurstFreqNew = 50.0
	cfg.NewTime = 0
	cfg.BurstHold = 50 * time.Millisecond
	cfg.BurstPenalty = 50 * time.Millisecond
	cfg.HeldReleaseInterval = 10 * time.Millisecond
	ic := NewIngressControlWith(cfg)

	for i := range 200 {
		ic.ProcessAnnounce("flood-"+itoa(i), []byte{byte(i)}, false)
	}
	if !ic.InBurst() {
		t.Fatal("expected burst-active after announce flood")
	}
	return ic
}

// clearBurst waits out the burst hold/penalty quiet period so held
// announces become releasable.
func clearBurst(t *testing.T, ic *IngressControl) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	ic.ProcessAnnounce("settle", []byte{0}, false)
	time.Sleep(150 * time.Millisecond)
	ic.ProcessAnnounce("settle2", []byte{0}, false)
	if ic.InBurst() {
		t.Fatal("expected burst cleared")
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_MatchesDest(t *testing.T) {
	ic := newBurstingIngress(t)
	dA := destAnnounceKey("dest-a")
	dB := destAnnounceKey("dest-b")
	dC := destAnnounceKey("dest-c")
	hashB := announceHoldKey("hb")

	ic.ProcessAnnounceHashDest(announceHoldKey("ha"), []byte("aaa"), true, dA)
	ic.ProcessAnnounceHashDest(hashB, []byte("bbb"), true, dB)
	ic.ProcessAnnounceHashDest(announceHoldKey("hc"), []byte("ccc"), true, dC)
	if got := ic.HeldCount(); got != 3 {
		t.Fatalf("HeldCount = %d, want 3", got)
	}
	clearBurst(t, ic)

	// dest-b sits mid-queue; the priority release must find it anyway.
	gotHash, gotData, ok := ic.ReleaseHeldAnnounceFor(dB)
	if !ok {
		t.Fatal("ReleaseHeldAnnounceFor should release the announce held for dest-b")
	}
	if gotHash != string(hashB[:]) {
		t.Errorf("released hash = %q, want %q", gotHash, string(hashB[:]))
	}
	if string(gotData) != "bbb" {
		t.Errorf("released data = %q, want %q", string(gotData), "bbb")
	}
	if got := ic.HeldCount(); got != 2 {
		t.Errorf("HeldCount = %d, want 2 after priority release", got)
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_PreservesFIFOOrder(t *testing.T) {
	ic := newBurstingIngress(t)
	dA := destAnnounceKey("dest-a")
	dB := destAnnounceKey("dest-b")
	dC := destAnnounceKey("dest-c")
	hashA := announceHoldKey("ha")
	hashC := announceHoldKey("hc")

	ic.ProcessAnnounceHashDest(hashA, []byte("aaa"), true, dA)
	ic.ProcessAnnounceHashDest(announceHoldKey("hb"), []byte("bbb"), true, dB)
	ic.ProcessAnnounceHashDest(hashC, []byte("ccc"), true, dC)
	clearBurst(t, ic)

	if _, _, ok := ic.ReleaseHeldAnnounceFor(dB); !ok {
		t.Fatal("priority release of dest-b should succeed")
	}

	gotHash, _, ok := ic.ReleaseHeldAnnounce()
	if !ok {
		t.Fatal("FIFO release should succeed")
	}
	if gotHash != string(hashA[:]) {
		t.Errorf("FIFO head = %q, want %q", gotHash, string(hashA[:]))
	}

	time.Sleep(15 * time.Millisecond)
	gotHash, _, ok = ic.ReleaseHeldAnnounce()
	if !ok {
		t.Fatal("second FIFO release should succeed after HeldReleaseInterval")
	}
	if gotHash != string(hashC[:]) {
		t.Errorf("FIFO second = %q, want %q", gotHash, string(hashC[:]))
	}
	if got := ic.HeldCount(); got != 0 {
		t.Errorf("HeldCount = %d, want 0", got)
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_NoMatch(t *testing.T) {
	ic := newBurstingIngress(t)

	ic.ProcessAnnounceHashDest(announceHoldKey("ha"), []byte("aaa"), true, destAnnounceKey("dest-a"))
	ic.ProcessAnnounceHash(announceHoldKey("hz"), []byte("zzz"), true) // zero dest
	clearBurst(t, ic)

	if _, _, ok := ic.ReleaseHeldAnnounceFor(destAnnounceKey("dest-b")); ok {
		t.Error("ReleaseHeldAnnounceFor should fail for a dest with no queued announce")
	}
	if _, _, ok := ic.ReleaseHeldAnnounceFor([16]byte{}); ok {
		t.Error("ReleaseHeldAnnounceFor must never match the zero destination")
	}
	if got := ic.HeldCount(); got != 2 {
		t.Errorf("HeldCount = %d, want 2 (no entry removed)", got)
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_RespectsBurstGates(t *testing.T) {
	ic := newBurstingIngress(t)
	dA := destAnnounceKey("dest-a")
	ic.ProcessAnnounceHashDest(announceHoldKey("ha"), []byte("aaa"), true, dA)

	if _, _, ok := ic.ReleaseHeldAnnounceFor(dA); ok {
		t.Error("ReleaseHeldAnnounceFor must not release while burst is active")
	}

	// First settle stamps burstClearedAt but the burst flag stays set.
	time.Sleep(150 * time.Millisecond)
	ic.ProcessAnnounce("settle", []byte{0}, false)
	if _, _, ok := ic.ReleaseHeldAnnounceFor(dA); ok {
		t.Error("ReleaseHeldAnnounceFor must not release while burst flag still set")
	}

	// Second settle clears burstActive inside the BurstHold+BurstPenalty
	// quiet period (60ms > BurstHold 50ms, < 100ms total).
	time.Sleep(60 * time.Millisecond)
	ic.ProcessAnnounce("settle2", []byte{0}, false)
	if ic.InBurst() {
		t.Fatal("expected burst cleared")
	}
	if _, _, ok := ic.ReleaseHeldAnnounceFor(dA); ok {
		t.Error("ReleaseHeldAnnounceFor must respect BurstHold+BurstPenalty quiet period")
	}

	time.Sleep(80 * time.Millisecond)
	if _, _, ok := ic.ReleaseHeldAnnounceFor(dA); !ok {
		t.Error("ReleaseHeldAnnounceFor should succeed once quiet period elapsed")
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_IgnoresReleaseInterval(t *testing.T) {
	ic := newBurstingIngress(t)
	dA := destAnnounceKey("dest-a")
	dB := destAnnounceKey("dest-b")

	ic.ProcessAnnounceHashDest(announceHoldKey("ha"), []byte("aaa"), true, dA)
	ic.ProcessAnnounceHashDest(announceHoldKey("hb"), []byte("bbb"), true, dB)
	clearBurst(t, ic)

	if _, _, ok := ic.ReleaseHeldAnnounce(); !ok {
		t.Fatal("FIFO release should succeed first")
	}
	// HeldReleaseInterval is 10ms; an immediate priority release for a
	// different dest must not be paced by it.
	if _, _, ok := ic.ReleaseHeldAnnounceFor(dB); !ok {
		t.Error("ReleaseHeldAnnounceFor must ignore HeldReleaseInterval")
	}
	if got := ic.HeldCount(); got != 0 {
		t.Errorf("HeldCount = %d, want 0", got)
	}
}

func TestIngressControl_ReleaseHeldAnnounceFor_IndexConsistency(t *testing.T) {
	ic := newBurstingIngress(t)
	dA := destAnnounceKey("dest-a")
	dB := destAnnounceKey("dest-b")
	dC := destAnnounceKey("dest-c")
	hashA := announceHoldKey("ha")
	hashB := announceHoldKey("hb")
	hashC := announceHoldKey("hc")

	ic.ProcessAnnounceHashDest(hashA, []byte("aaa"), true, dA)
	ic.ProcessAnnounceHashDest(hashB, []byte("bbb"), true, dB)
	ic.ProcessAnnounceHashDest(hashC, []byte("ccc"), true, dC)
	clearBurst(t, ic)

	if _, _, ok := ic.ReleaseHeldAnnounceFor(dB); !ok {
		t.Fatal("priority release of dest-b should succeed")
	}

	// Re-burst so the hold path runs again.
	for i := range 200 {
		ic.ProcessAnnounce("reflood-"+itoa(i), []byte{byte(i)}, false)
	}
	if !ic.InBurst() {
		t.Fatal("expected burst-active after re-flood")
	}

	// hashC is still queued: resubmitting it is a duplicate.
	if ic.ProcessAnnounceHashDest(hashC, []byte("ccc2"), true, dC) {
		t.Fatal("resubmitted queued hash should be held")
	}
	if got := ic.HeldCount(); got != 2 {
		t.Fatalf("HeldCount = %d, want 2 after duplicate submit", got)
	}

	// hashB was priority-released: resubmitting queues it again at tail.
	if ic.ProcessAnnounceHashDest(hashB, []byte("bbb2"), true, dB) {
		t.Fatal("released hash should be held again")
	}
	if got := ic.HeldCount(); got != 3 {
		t.Fatalf("HeldCount = %d, want 3 after requeue", got)
	}

	// Queue order must be A, C, B after the clear and rebuild.
	clearBurst(t, ic)
	want := []string{string(hashA[:]), string(hashC[:]), string(hashB[:])}
	for i, w := range want {
		gotHash, _, ok := ic.ReleaseHeldAnnounce()
		if !ok {
			t.Fatalf("FIFO release %d should succeed", i)
		}
		if gotHash != w {
			t.Errorf("FIFO release %d = %q, want %q", i, gotHash, w)
		}
		if i < len(want)-1 {
			time.Sleep(15 * time.Millisecond)
		}
	}
	if got := ic.HeldCount(); got != 0 {
		t.Errorf("HeldCount = %d, want 0", got)
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
