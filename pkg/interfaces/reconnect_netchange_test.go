// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package interfaces

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// quarantinedTracker returns a tracker with a controlled address probe,
// drives label into quarantine, then establishes the probe baseline so
// later snapshot changes compare deterministically. cur holds the live
// snapshot map for the test to swap.
func quarantinedTracker(t *testing.T, label string, base map[string]struct{}) (*EndpointTracker, *atomic.Value) {
	t.Helper()
	et := NewEndpointTracker()
	cur := &atomic.Value{}
	cur.Store(base)
	et.netProbe = func() (map[string]struct{}, error) {
		return cur.Load().(map[string]struct{}), nil
	}
	for i := 0; i < EndpointQuarantineConsecutiveFails; i++ {
		et.RecordFailure(label)
	}
	if ok, _ := et.DialAllowed(label); ok {
		t.Fatal("expected quarantine")
	}
	et.NetworkChanged()
	return et, cur
}

func quarantinedDriver(label string, et *EndpointTracker, done chan struct{}, dials *atomic.Int32) *reconnectDriver {
	rd := newReconnectDriver(label, -1, done, func() (net.Conn, error) {
		dials.Add(1)
		return nil, errors.New("refused")
	}, func(net.Conn) {})
	rd.tracker = et
	return rd
}

// TestReconnectKickNetworkUpClearsQuarantine verifies a netwatch kick after
// an interface gains an address resets quarantine and dials immediately.
func TestReconnectKickNetworkUpClearsQuarantine(t *testing.T) {
	fire, subs, _ := stubNetwatch(t)
	label := "wifi-down:4242"
	base := map[string]struct{}{"eth0|192.168.1.2": {}}
	et, cur := quarantinedTracker(t, label, base)

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	rd := quarantinedDriver(label, et, done, dials)
	rd.start()
	waitForAtom(t, 3*time.Second, func() bool { return subs.Load() > 0 }, "netwatch subscription")

	time.Sleep(150 * time.Millisecond)
	if d := dials.Load(); d != 0 {
		t.Fatalf("dial func invoked while quarantined (dials=%d)", d)
	}

	cur.Store(map[string]struct{}{
		"eth0|192.168.1.2": {},
		"wlan0|10.0.0.5":   {},
	})
	fire()
	waitForAtom(t, 1500*time.Millisecond, func() bool { return dials.Load() > 0 }, "dial after network-up kick")
}

// TestReconnectPollNetworkUpClearsQuarantine verifies the quarantine wait
// slices re-probe interface addresses, so a network-up resets quarantine
// even without a netwatch event.
func TestReconnectPollNetworkUpClearsQuarantine(t *testing.T) {
	orig := quarantinePollInterval
	quarantinePollInterval = 50 * time.Millisecond
	t.Cleanup(func() { quarantinePollInterval = orig })

	_, subs, _ := stubNetwatch(t)
	label := "wifi-poll:4242"
	base := map[string]struct{}{"eth0|192.168.1.2": {}}
	et, cur := quarantinedTracker(t, label, base)

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	rd := quarantinedDriver(label, et, done, dials)
	rd.start()
	waitForAtom(t, 3*time.Second, func() bool { return subs.Load() > 0 }, "netwatch subscription")

	cur.Store(map[string]struct{}{
		"eth0|192.168.1.2": {},
		"wlan0|10.0.0.5":   {},
	})
	waitForAtom(t, 1500*time.Millisecond, func() bool { return dials.Load() > 0 }, "dial after poll-detected network-up")
}

// TestReconnectKickRemovalKeepsQuarantine verifies an address-removal event
// does not reset quarantine or force a dial.
func TestReconnectKickRemovalKeepsQuarantine(t *testing.T) {
	fire, subs, _ := stubNetwatch(t)
	label := "wifi-gone:4242"
	base := map[string]struct{}{
		"eth0|192.168.1.2": {},
		"wlan0|10.0.0.5":   {},
	}
	et, cur := quarantinedTracker(t, label, base)

	done := make(chan struct{})
	defer close(done)
	dials := &atomic.Int32{}
	rd := quarantinedDriver(label, et, done, dials)
	rd.start()
	waitForAtom(t, 3*time.Second, func() bool { return subs.Load() > 0 }, "netwatch subscription")

	cur.Store(map[string]struct{}{"eth0|192.168.1.2": {}})
	fire()
	time.Sleep(300 * time.Millisecond)
	if d := dials.Load(); d != 0 {
		t.Fatalf("removal-only network change forced a dial (dials=%d)", d)
	}
	if ok, _ := et.DialAllowed(label); ok {
		t.Fatal("removal-only network change cleared quarantine")
	}
}

// TestReconnectQuarantineWaitAbortsOnShutdown verifies closing done while
// the driver waits out a quarantine returns promptly without a leaked
// goroutine.
func TestReconnectQuarantineWaitAbortsOnShutdown(t *testing.T) {
	_, subs, _ := stubNetwatch(t)
	label := "wifi-abort:4242"
	base := map[string]struct{}{"eth0|192.168.1.2": {}}
	et, _ := quarantinedTracker(t, label, base)

	done := make(chan struct{})
	dials := &atomic.Int32{}
	rd := quarantinedDriver(label, et, done, dials)
	rd.start()
	waitForAtom(t, 3*time.Second, func() bool { return subs.Load() > 0 }, "netwatch subscription")
	time.Sleep(100 * time.Millisecond)

	close(done)
	waitForAtom(t, 2*time.Second, func() bool { return !rd.isActive() }, "driver exit after shutdown")
	if d := dials.Load(); d != 0 {
		t.Fatalf("dial func invoked while quarantined (dials=%d)", d)
	}
}
