// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package interop

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/health"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/protect"
	"quad4/reticulum-go/tests/interop/harness"
)

// TestLiveDoSProtectionQuietNoSpuriousTrips runs under RUN_LIVE_INTEROP=1.
func TestLiveDoSProtectionQuietNoSpuriousTrips(t *testing.T) {
	liveOrSkip(t)
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModeDetect,
		MaxPPS:       protect.DefaultMaxPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			if e.TripCount(protect.ReasonPPS) != 0 {
				t.Fatalf("spurious pps trips under quiet live traffic")
			}
			return
		case <-ticker.C:
			_ = e.AdmitPacket("live0", 128)
		}
	}
}

// TestLiveDoSProtectUDPIfaceQuietAndFlood exercises real UDP sockets end to end.
func TestLiveDoSProtectUDPIfaceQuietAndFlood(t *testing.T) {
	liveOrSkip(t)
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	a, err := interfaces.NewUDPInterface("dos_a", fmt.Sprintf("127.0.0.1:%d", portA), fmt.Sprintf("127.0.0.1:%d", portB), true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := interfaces.NewUDPInterface("dos_b", fmt.Sprintf("127.0.0.1:%d", portB), fmt.Sprintf("127.0.0.1:%d", portA), true)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:            protect.ModePrevent,
		MaxPPS:          60,
		WarnWriter:      &buf,
		WarnInterval:    time.Hour,
		DisableAdaptive: true,
		DisableCoolDown: true,
	})
	protect.SetDefault(e)

	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		got.Add(1)
	})
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	payload := []byte("quiet-live")
	for range 20 {
		if err := a.ProcessOutgoing(payload); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	waitUntil(t, 2*time.Second, func() bool { return got.Load() >= 15 })
	quietTrips := e.TripCount(protect.ReasonPPS)
	if quietTrips != 0 {
		t.Fatalf("quiet false positive trips=%d", quietTrips)
	}

	got.Store(0)
	flood := make([]byte, 64)
	for range 500 {
		_ = a.ProcessOutgoing(flood)
	}
	time.Sleep(300 * time.Millisecond)
	if got.Load() >= 500 {
		t.Fatalf("flood should shed got=%d", got.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("flood expected pps trips")
	}
}

// TestLiveDoSProtectAutoLearnOnUDP promotes on quiet real traffic then sheds flood.
func TestLiveDoSProtectAutoLearnOnUDP(t *testing.T) {
	liveOrSkip(t)
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()

	dir := t.TempDir()
	store := filepath.Join(dir, protect.StoreFileName)
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:                 protect.ModeAuto,
		MaxPPS:               protect.DefaultMaxPPS,
		FloorPPS:             40,
		StorePath:            store,
		WarnWriter:           &buf,
		WarnInterval:         time.Hour,
		DisableCoolDown:      true,
		AutoLearnMinDuration: 2 * time.Second,
		AutoLearnMinSamples:  8,
	})
	protect.SetDefault(e)
	e.NotifyInterfaces([]string{"auto_live"})

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	a, err := interfaces.NewUDPInterface("auto_src", fmt.Sprintf("127.0.0.1:%d", portA), fmt.Sprintf("127.0.0.1:%d", portB), true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := interfaces.NewUDPInterface("auto_live", fmt.Sprintf("127.0.0.1:%d", portB), fmt.Sprintf("127.0.0.1:%d", portA), true)
	if err != nil {
		t.Fatal(err)
	}
	var got atomic.Int64
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) { got.Add(1) })
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()

	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) && e.Phase() != protect.AutoArmed {
		_ = a.ProcessOutgoing(payload)
		time.Sleep(250 * time.Millisecond)
	}
	if e.Phase() != protect.AutoArmed {
		t.Fatalf("auto did not arm on live quiet traffic phase=%s warn=%q samples=%v", e.Phase(), buf.String(), baselineSamples(e, "auto_live"))
	}
	if _, err := os.Stat(store); err != nil {
		t.Fatalf("expected persist store: %v", err)
	}

	got.Store(0)
	flood := make([]byte, 48)
	for range 500 {
		_ = a.ProcessOutgoing(flood)
	}
	time.Sleep(300 * time.Millisecond)
	if e.Phase() != protect.AutoArmed {
		t.Fatalf("flood demoted auto phase=%s", e.Phase())
	}
	if got.Load() >= 500 {
		t.Fatalf("armed auto flood should shed got=%d", got.Load())
	}
	if e.TripCount(protect.ReasonPPS) == 0 {
		t.Fatal("expected flood trips after arming")
	}
}

// TestLiveDoSProtectTransportUDPQuiet wires protect through a live Transport + UDP iface.
func TestLiveDoSProtectTransportUDPQuiet(t *testing.T) {
	liveOrSkip(t)
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()

	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxPPS:       protect.DefaultMaxPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	portA := freeUDPPort(t)
	portB := freeUDPPort(t)
	trA, ifaceA, cleanupA := setupGoUDPPeer(t, portA, portB)
	defer cleanupA()
	trB, ifaceB, cleanupB := setupGoUDPPeer(t, portB, portA)
	defer cleanupB()
	_ = trA
	_ = trB
	_ = ifaceA

	payload := make([]byte, 32)
	for range 40 {
		ifaceB.ProcessIncoming(payload)
		time.Sleep(30 * time.Millisecond)
	}
	if e.TripCount(protect.ReasonPPS) != 0 {
		t.Fatalf("transport quiet false positive trips=%d", e.TripCount(protect.ReasonPPS))
	}

	// Real datagram on the paired UDP path confirms sockets are live.
	var got atomic.Int64
	prev := ifaceB.GetPacketCallback()
	ifaceB.SetPacketCallback(func(data []byte, iface common.NetworkInterface) {
		got.Add(1)
		if prev != nil {
			prev(data, iface)
		}
	})
	if err := ifaceA.ProcessOutgoing(payload); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return got.Load() >= 1 })
}

func baselineSamples(e *protect.Engine, iface string) int {
	_, _, samples, _ := e.IfaceBaseline(iface)
	return samples
}

// TestLiveDoSProtectMeshPeersDialQuiet dials real public mesh TCP peers when reachable.
func TestLiveDoSProtectMeshPeersDialQuiet(t *testing.T) {
	liveOrSkip(t)
	t.Cleanup(func() { protect.SetDefault(nil) })
	health.Default.Reset()

	online, err := harness.MeshPreflight(nil, 3*time.Second)
	if err != nil {
		t.Skipf("mesh peers unreachable: %v", err)
	}

	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxConns:     64,
		MaxPPS:       protect.DefaultMaxPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	protect.SetDefault(e)

	// Local TCP server with protect, plus outbound dials to mesh peers.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	srv, err := interfaces.NewTCPServerInterface("mesh_protect", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	var localHeld []net.Conn
	defer func() {
		for _, c := range localHeld {
			_ = c.Close()
		}
	}()
	for range 3 {
		c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
		if err != nil {
			t.Fatalf("local dial: %v", err)
		}
		localHeld = append(localHeld, c)
	}

	peers := harness.DefaultMeshPeers()
	reachable := 0
	for _, p := range peers {
		addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		c, dialErr := net.DialTimeout("tcp", addr, 3*time.Second)
		if dialErr != nil {
			continue
		}
		_ = c.Close()
		reachable++
		// Outbound mesh dials are not AdmitConn on our server. Record via packet gate.
		_ = e.AdmitPacket("mesh_out", 64)
	}
	if reachable == 0 {
		t.Skip("no mesh peers accepted TCP")
	}
	if e.TripCount(protect.ReasonPPS) != 0 || e.TripCount(protect.ReasonConn) != 0 {
		t.Fatalf("mesh quiet false trips pps=%d conn=%d online=%d",
			e.TripCount(protect.ReasonPPS), e.TripCount(protect.ReasonConn), online)
	}
	t.Logf("mesh preflight online=%d dialed=%d protect clean", online, reachable)
}

// TestLiveDoSProtectFalsePositiveBudget runs a longer quiet wall-clock budget.
func TestLiveDoSProtectFalsePositiveBudget(t *testing.T) {
	liveOrSkip(t)
	secs := 8
	if v := os.Getenv("PROTECT_LIVE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 3 {
			secs = min(n, 60)
		}
	}
	var buf bytes.Buffer
	e := protect.New(protect.Options{
		Mode:         protect.ModePrevent,
		MaxPPS:       protect.DefaultMaxPPS,
		FloorPPS:     protect.DefaultFloorPPS,
		WarnWriter:   &buf,
		WarnInterval: time.Hour,
	})
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(time.Duration(secs) * time.Second)
	allowed := 0
	for {
		select {
		case <-deadline:
			if e.TripCount(protect.ReasonPPS) != 0 || e.TripCount(protect.ReasonBPS) != 0 {
				t.Fatalf("FP budget exceeded trips pps=%d bps=%d allowed=%d",
					e.TripCount(protect.ReasonPPS), e.TripCount(protect.ReasonBPS), allowed)
			}
			return
		case <-ticker.C:
			d := e.AdmitPacket("budget0", 96)
			if !d.Allow {
				t.Fatalf("false block under quiet budget %#v", d)
			}
			allowed++
		}
	}
}

func waitUntil(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !ok() {
		t.Fatalf("condition not met within %s", d)
	}
}
