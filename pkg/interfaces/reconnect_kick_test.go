// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitForAtom polls cond until it holds or the deadline passes.
func waitForAtom(t *testing.T, d time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestJitterBackoffBounds(t *testing.T) {
	base := 4 * time.Second
	for range 2000 {
		got := jitterBackoff(base)
		if got < base/2 || got >= base {
			t.Fatalf("jitterBackoff(%v) = %v outside [base/2, base)", base, got)
		}
	}
	if got := jitterBackoff(0); got != 0 {
		t.Fatalf("jitterBackoff(0) = %v", got)
	}
	if got := jitterBackoff(time.Millisecond); got != time.Millisecond {
		t.Fatalf("jitterBackoff(1ms) = %v", got)
	}
}

// stubNetwatch replaces netwatchSubscribe with a manual trigger and returns a
// trigger function plus counters for subscription events.
func stubNetwatch(t *testing.T) (fire func(), subs *atomic.Int32, unsubs *atomic.Int32) {
	t.Helper()
	subs = &atomic.Int32{}
	unsubs = &atomic.Int32{}
	var mu sync.Mutex
	var cbs []func()
	orig := netwatchSubscribe
	netwatchSubscribe = func(fn func()) func() {
		subs.Add(1)
		active := true
		cb := func() {
			mu.Lock()
			ok := active
			mu.Unlock()
			if ok {
				fn()
			}
		}
		mu.Lock()
		cbs = append(cbs, cb)
		mu.Unlock()
		return func() {
			mu.Lock()
			active = false
			mu.Unlock()
			unsubs.Add(1)
		}
	}
	t.Cleanup(func() { netwatchSubscribe = orig })
	return func() {
		mu.Lock()
		snapshot := append([]func(){}, cbs...)
		mu.Unlock()
		for _, fn := range snapshot {
			fn()
		}
	}, subs, unsubs
}

func TestReconnectSubscribesToNetwatch(t *testing.T) {
	fire, subs, unsubs := stubNetwatch(t)
	_ = fire

	done := make(chan struct{})
	dials := &atomic.Int32{}
	rd := newReconnectDriver("t", -1, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("dial failed")
	}, func(net.Conn) {})

	rd.start()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && subs.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if subs.Load() == 0 {
		t.Fatal("reconnect driver never subscribed to netwatch")
	}
	close(done)
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && rd.isActive() {
		time.Sleep(5 * time.Millisecond)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && unsubs.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if unsubs.Load() == 0 {
		t.Fatal("reconnect driver did not unsubscribe on run exit")
	}
}

// TestReconnectKickWakesBackoff verifies a network-change kick interrupts the
// backoff sleep so a reconnect attempt runs far earlier than the timer.
func TestReconnectKickWakesBackoff(t *testing.T) {
	fire, subs, _ := stubNetwatch(t)

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	rd := newReconnectDriver("t", -1, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("dial failed")
	}, func(net.Conn) {})

	rd.start()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && subs.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	// First dial has failed. Driver is now sleeping ~1s jittered.
	for time.Now().Before(deadline) && dials.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if dials.Load() == 0 {
		t.Fatal("dial never attempted")
	}

	fire()

	deadline = time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) && dials.Load() < 2 {
		time.Sleep(2 * time.Millisecond)
	}
	if dials.Load() < 2 {
		t.Fatal("kick did not wake reconnect before backoff elapsed")
	}
}

// TestReconnectKickRateLimited verifies the second kick inside the rate-limit
// window is dropped and backoff timing is not shortened again.
func TestReconnectKickRateLimited(t *testing.T) {
	fire, subs, _ := stubNetwatch(t)

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	rd := newReconnectDriver("t", -1, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("dial failed")
	}, func(net.Conn) {})

	rd.start()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && subs.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	for time.Now().Before(deadline) && dials.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	t0 := time.Now()
	fire()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && dials.Load() < 2 {
		time.Sleep(2 * time.Millisecond)
	}
	if dials.Load() < 2 {
		t.Fatal("first kick did not wake the driver")
	}
	if time.Since(t0) > 500*time.Millisecond {
		t.Skip("scheduler too slow to exercise the rate-limit window")
	}

	// Second kick inside kickMinInterval must not refill the wake channel.
	fire()
	if dials.Load() >= 3 {
		t.Fatal("second kick raced the third dial. Rate limit not applied")
	}
	// jittered backoff floor is InitialBackoff/2 = 500ms. A dropped kick
	// must not produce a dial sooner than that.
	time.Sleep(450 * time.Millisecond)
	if d := dials.Load(); d >= 3 {
		t.Fatalf("rate-limited kick still forced a dial (dials=%d)", d)
	}
}

// TestReconnectNeverDialsOnceOnly verifies max_reconnect_tries = 0 dials the
// initial connection once, fires exhausted on failure, and never retries.
// Regression: the -2 sentinel used to fall into the negative=unlimited
// branch, reconnecting forever.
func TestReconnectNeverDialsOnceOnly(t *testing.T) {
	fire, _, _ := stubNetwatch(t)
	_ = fire

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	exhausted := &atomic.Int32{}
	rd := newReconnectDriver("t", ReconnectNever, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("dial failed")
	}, func(net.Conn) {})
	rd.setOnExhausted(func() { exhausted.Add(1) })

	rd.start()
	waitForAtom(t, 3*time.Second, func() bool { return exhausted.Load() > 0 }, "exhausted callback")
	if d := dials.Load(); d != 1 {
		t.Fatalf("ReconnectNever dialed %d times, want exactly 1", d)
	}

	// A later failure notification must not dial again while never connected.
	rd.notifyFailure()
	time.Sleep(300 * time.Millisecond)
	if d := dials.Load(); d != 1 {
		t.Fatalf("ReconnectNever redialed after failure notification (dials=%d)", d)
	}
}

// TestReconnectNeverNoRetryAfterConnect verifies that once a session was
// established, a later drop produces no reconnect attempts at all.
func TestReconnectNeverNoRetryAfterConnect(t *testing.T) {
	fire, _, _ := stubNetwatch(t)
	_ = fire

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	connected := make(chan net.Conn, 1)
	rd := newReconnectDriver("t", ReconnectNever, done, func() (net.Conn, error) {
		dials.Add(1)
		c1, c2 := net.Pipe()
		_ = c2
		return c1, nil
	}, func(c net.Conn) { connected <- c })

	rd.start()
	select {
	case c := <-connected:
		_ = c.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("initial connect never delivered")
	}
	if d := dials.Load(); d != 1 {
		t.Fatalf("ReconnectNever dialed %d times, want 1", d)
	}

	// Simulate the established connection dropping.
	rd.notifyFailure()
	time.Sleep(300 * time.Millisecond)
	if d := dials.Load(); d != 1 {
		t.Fatalf("ReconnectNever redialed after established session dropped (dials=%d)", d)
	}
}
