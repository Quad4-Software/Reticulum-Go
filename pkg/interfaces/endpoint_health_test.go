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

func TestEndpointTrackerAllowsInitially(t *testing.T) {
	et := NewEndpointTracker()
	if ok, _ := et.DialAllowed("h:1"); !ok {
		t.Fatal("fresh endpoint should be dialable")
	}
}

func TestEndpointTrackerQuarantinesAfterFails(t *testing.T) {
	et := NewEndpointTracker()
	ep := "h:2"
	for i := 0; i < EndpointQuarantineConsecutiveFails-1; i++ {
		et.RecordFailure(ep)
		if ok, _ := et.DialAllowed(ep); !ok {
			t.Fatalf("quarantined early at %d fails", i+1)
		}
	}
	et.RecordFailure(ep)
	ok, wait := et.DialAllowed(ep)
	if ok {
		t.Fatal("endpoint not quarantined at threshold")
	}
	if wait <= 0 || wait > EndpointQuarantineBase {
		t.Fatalf("quarantine wait %v out of bounds", wait)
	}
}

func TestEndpointTrackerEscalates(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	ep := "h:3"

	quarantine := func() time.Duration {
		for i := 0; i < EndpointQuarantineConsecutiveFails; i++ {
			et.RecordFailure(ep)
		}
		ok, wait := et.DialAllowed(ep)
		if ok {
			t.Fatal("expected quarantine")
		}
		return wait
	}

	first := quarantine()
	// Wait out the quarantine, then trigger a second.
	now = now.Add(first + time.Second)
	second := quarantine()
	if second <= first {
		t.Fatalf("quarantine should escalate: first=%v second=%v", first, second)
	}
}

func TestEndpointTrackerSuccessRecovers(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	ep := "h:4"

	for i := 0; i < EndpointQuarantineConsecutiveFails; i++ {
		et.RecordFailure(ep)
	}
	if ok, _ := et.DialAllowed(ep); ok {
		t.Fatal("expected quarantine")
	}
	now = now.Add(EndpointQuarantineBase + time.Second)
	// After the cooldown elapses a dial is allowed; success halves escalation.
	et.RecordSuccess(ep)
	if ok, _ := et.DialAllowed(ep); !ok {
		t.Fatal("endpoint should be dialable after cooldown")
	}
	st := et.Status(ep)
	if !st.Tracked || st.Successes != 1 {
		t.Fatalf("status=%+v", st)
	}
}

func TestEndpointTrackerFlapBurstQuarantines(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	ep := "h:5"
	for i := 0; i < EndpointFlapBurst; i++ {
		et.RecordFlap(ep)
	}
	ok, _ := et.DialAllowed(ep)
	if ok {
		t.Fatal("flap burst should quarantine")
	}
}

func TestEndpointTrackerFlapWindowAges(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	ep := "h:6"
	for i := 0; i < EndpointFlapBurst-1; i++ {
		et.RecordFlap(ep)
		now = now.Add(EndpointFlapWindow + time.Second)
	}
	if ok, _ := et.DialAllowed(ep); !ok {
		t.Fatal("stale flaps should not quarantine")
	}
}

func TestEndpointTrackerStatusUntracked(t *testing.T) {
	et := NewEndpointTracker()
	st := et.Status("never-seen:1")
	if st.Tracked {
		t.Fatal("untracked endpoint should report Tracked=false")
	}
}

// TestReconnectDriverQuarantineDelay verifies dialTracked accounting:
// sustained failures quarantine the endpoint and block further dials until
// the cooldown elapses.
func TestReconnectDriverQuarantineDelay(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	done := make(chan struct{})

	var dials atomic.Int32
	ep := "dead:4242"
	rd := newReconnectDriver(ep, EndpointQuarantineConsecutiveFails+2, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("refused")
	}, func(net.Conn) {})
	rd.tracker = et

	for i := 0; i < EndpointQuarantineConsecutiveFails; i++ {
		if _, err := rd.dialTracked(); err == nil {
			t.Fatal("expected dial error")
		}
	}
	st := et.Status(ep)
	if !st.Quarantined {
		t.Fatalf("endpoint not quarantined; status=%+v", st)
	}
	if st.DialFailures != uint64(EndpointQuarantineConsecutiveFails) {
		t.Fatalf("dial_failures=%d want %d", st.DialFailures, EndpointQuarantineConsecutiveFails)
	}

	// While quarantined, a dial attempt must not reach the dial func. The
	// wait aborts cleanly on driver shutdown.
	before := dials.Load()
	waitDone := make(chan error, 1)
	go func() {
		_, err := rd.dialTracked()
		waitDone <- err
	}()
	select {
	case <-waitDone:
		t.Fatal("dialTracked returned while quarantined")
	case <-time.After(200 * time.Millisecond):
	}
	if dials.Load() != before {
		t.Fatal("dial func invoked during quarantine")
	}
	close(done)
	select {
	case err := <-waitDone:
		if !errors.Is(err, errDialAborted) {
			t.Fatalf("dialTracked abort err=%v want errDialAborted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dialTracked did not abort on shutdown")
	}

	// After the cooldown elapses the endpoint is dialable again.
	now = now.Add(EndpointQuarantineBase + time.Second)
	if ok, _ := et.DialAllowed(ep); !ok {
		t.Fatal("endpoint still quarantined after cooldown")
	}
}

// TestReconnectDriverRecordsFlap verifies a fast connect-drop counts a flap.
func TestReconnectDriverRecordsFlap(t *testing.T) {
	et := NewEndpointTracker()
	done := make(chan struct{})
	defer close(done)
	ep := "flap:4242"
	conn := &fakeConn{}
	rd := newReconnectDriver(ep, -1, done, func() (net.Conn, error) { return conn, nil }, func(net.Conn) {})
	rd.tracker = et

	// Dial once via dialTracked so connectedAt is set.
	c, err := rd.dialTracked()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if c == nil {
		t.Fatal("nil conn")
	}
	rd.notifyFailure()
	st := et.Status(ep)
	if st.Flaps != 1 {
		t.Fatalf("flaps=%d want 1", st.Flaps)
	}
	if st.Successes != 1 {
		t.Fatalf("successes=%d want 1", st.Successes)
	}
}

// TestReconnectDriverSlowDropNotFlap: a long-lived conn drop is not a flap.
func TestReconnectDriverSlowDropNotFlap(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	done := make(chan struct{})
	defer close(done)
	ep := "slow:4242"
	conn := &fakeConn{}
	rd := newReconnectDriver(ep, -1, done, func() (net.Conn, error) { return conn, nil }, func(net.Conn) {})
	rd.tracker = et
	rd.tracker.now = func() time.Time { return now }

	if _, err := rd.dialTracked(); err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Advance past the flap lifetime before the drop.
	now = now.Add(EndpointFlapLifetime + time.Second)
	rd.notifyFailure()
	st := et.Status(ep)
	if st.Flaps != 0 {
		t.Fatalf("slow drop counted as flap: %d", st.Flaps)
	}
}

type fakeConn struct{ net.Conn }

func (f *fakeConn) Close() error { return nil }

// quarantineNow drives ep into quarantine at the tracker's current clock.
func quarantineNow(t *testing.T, et *EndpointTracker, ep string) time.Duration {
	t.Helper()
	for i := 0; i < EndpointQuarantineConsecutiveFails; i++ {
		et.RecordFailure(ep)
	}
	ok, wait := et.DialAllowed(ep)
	if ok {
		t.Fatal("expected quarantine")
	}
	return wait
}

func TestEndpointTrackerResetQuarantines(t *testing.T) {
	et := NewEndpointTracker()
	now := time.Now()
	et.now = func() time.Time { return now }
	ep := "reset:1"

	first := quarantineNow(t, et, ep)
	now = now.Add(first + time.Second)
	second := quarantineNow(t, et, ep)
	if second <= first {
		t.Fatal("expected escalation before reset")
	}

	et.RecordFlap(ep)
	et.ResetQuarantines()

	if ok, _ := et.DialAllowed(ep); !ok {
		t.Fatal("endpoint still quarantined after reset")
	}
	st := et.Status(ep)
	if st.DialFailures != 2*EndpointQuarantineConsecutiveFails {
		t.Fatalf("dial_failures=%d, lifetime counters must survive reset", st.DialFailures)
	}
	if st.Flaps != 1 {
		t.Fatalf("flaps=%d, lifetime counters must survive reset", st.Flaps)
	}

	// Escalation restarts at the base duration after a reset.
	third := quarantineNow(t, et, ep)
	if third > EndpointQuarantineBase || third <= EndpointQuarantineBase/2 {
		t.Fatalf("post-reset quarantine %v, want about %v", third, EndpointQuarantineBase)
	}
}

func TestEndpointTrackerNilSafeReset(t *testing.T) {
	var et *EndpointTracker
	et.ResetQuarantines()
	if et.NetworkChanged() {
		t.Fatal("nil tracker must report no change")
	}
}

// probeTracker returns a tracker whose address snapshot is controlled by the
// returned setter.
func probeTracker(t *testing.T) (*EndpointTracker, func(map[string]struct{}, error)) {
	t.Helper()
	et := NewEndpointTracker()
	var mu sync.Mutex
	cur := map[string]struct{}{"eth0|192.168.1.2": {}}
	var err error
	et.netProbe = func() (map[string]struct{}, error) {
		mu.Lock()
		defer mu.Unlock()
		return cur, err
	}
	return et, func(m map[string]struct{}, e error) {
		mu.Lock()
		cur = m
		err = e
		mu.Unlock()
	}
}

func TestEndpointTrackerNetworkChangedBaseline(t *testing.T) {
	et, _ := probeTracker(t)
	ep := "base:1"
	quarantineNow(t, et, ep)
	// First call only establishes the baseline: no reset, returns false.
	if et.NetworkChanged() {
		t.Fatal("baseline call must return false")
	}
	if ok, _ := et.DialAllowed(ep); ok {
		t.Fatal("baseline call must not clear quarantine")
	}
}

func TestEndpointTrackerNetworkChangedGainedResets(t *testing.T) {
	et, set := probeTracker(t)
	ep := "gain:1"
	quarantineNow(t, et, ep)
	if et.NetworkChanged() {
		t.Fatal("baseline call must return false")
	}
	set(map[string]struct{}{
		"eth0|192.168.1.2": {},
		"wlan0|10.0.0.5":   {},
	}, nil)
	if !et.NetworkChanged() {
		t.Fatal("gained address must reset and return true")
	}
	if ok, _ := et.DialAllowed(ep); !ok {
		t.Fatal("gained address must clear quarantine")
	}
	// Identical snapshot is a no-op.
	if et.NetworkChanged() {
		t.Fatal("unchanged snapshot must return false")
	}
}

func TestEndpointTrackerNetworkChangedRemovalOnly(t *testing.T) {
	et, set := probeTracker(t)
	ep := "lose:1"
	quarantineNow(t, et, ep)
	et.NetworkChanged()
	set(map[string]struct{}{}, nil)
	if et.NetworkChanged() {
		t.Fatal("removal-only change must not reset")
	}
	if ok, _ := et.DialAllowed(ep); ok {
		t.Fatal("removal-only change must keep quarantine")
	}
}

func TestEndpointTrackerNetworkChangedProbeError(t *testing.T) {
	et, set := probeTracker(t)
	ep := "err:1"
	quarantineNow(t, et, ep)
	et.NetworkChanged()
	set(nil, errors.New("probe failed"))
	if et.NetworkChanged() {
		t.Fatal("probe error must return false")
	}
	if ok, _ := et.DialAllowed(ep); ok {
		t.Fatal("probe error must keep quarantine")
	}
	// The failed probe did not replace the baseline: a later gain still
	// resets against the last good snapshot.
	set(map[string]struct{}{
		"eth0|192.168.1.2": {},
		"wlan0|10.0.0.5":   {},
	}, nil)
	if !et.NetworkChanged() {
		t.Fatal("gain after probe error must reset")
	}
}

func TestEndpointTrackerNetworkChangedConcurrent(t *testing.T) {
	et, set := probeTracker(t)
	ep := "race:1"
	et.NetworkChanged()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			set(map[string]struct{}{"eth0|192.168.1.2": {}}, nil)
			set(map[string]struct{}{
				"eth0|192.168.1.2": {},
				"wlan0|10.0.0.5":   {},
			}, nil)
		}
	}()
	for i := 0; i < 200; i++ {
		et.NetworkChanged()
		et.RecordFailure(ep)
		et.DialAllowed(ep)
		et.ResetQuarantines()
	}
	<-done
}
