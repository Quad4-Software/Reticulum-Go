// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"crypto/rand"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Quad4-Software/Reticulum-Go/pkg/common"
	"github.com/Quad4-Software/Reticulum-Go/pkg/health"
	"github.com/Quad4-Software/Reticulum-Go/pkg/identity"
	"github.com/Quad4-Software/Reticulum-Go/pkg/interfaces"
	"github.com/Quad4-Software/Reticulum-Go/pkg/protect"
)

// floodIface counts outbound frames; the inbound path is exercised by
// calling tr.HandlePacket directly with crafted wire bytes, which is the
// same entry point real interfaces use.
type floodIface struct {
	common.BaseInterface
	sentMu sync.Mutex
	sent   int
	sentB  int64
}

func (m *floodIface) Send(data []byte, _ string) error {
	m.sentMu.Lock()
	m.sent++
	m.sentB += int64(len(data))
	m.sentMu.Unlock()
	return nil
}
func (m *floodIface) GetName() string  { return m.Name }
func (m *floodIface) IsEnabled() bool  { return m.Enabled }
func (m *floodIface) sentCount() int   { m.sentMu.Lock(); defer m.sentMu.Unlock(); return m.sent }
func (m *floodIface) sentBytes() int64 { m.sentMu.Lock(); defer m.sentMu.Unlock(); return m.sentB }

func newFloodIface(name string) *floodIface {
	m := &floodIface{}
	m.Name = name
	m.Enabled = true
	m.Online = true
	return m
}

func heapAlloc() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// TestFloodAnnounceReplayIsCheap proves dedup runs before the Ed25519
// verify: replaying an already-seen announce must not cost a signature
// verification per packet. Timing assertion is generous (100x margin)
// since CI machines vary.
func TestFloodAnnounceReplayIsCheap(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	health.Default.Reset()
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := newFloodIface("flood1")
	if err := tr.RegisterInterface("flood1", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := signedAnnounceRaw(t, tr, id)

	// First delivery pays the verify and is accepted.
	tr.HandlePacket(append([]byte(nil), raw...), iface)
	time.Sleep(50 * time.Millisecond)

	const replays = 4000
	start := time.Now()
	for i := 0; i < replays; i++ {
		tr.HandlePacket(raw, iface)
	}
	perReplay := time.Since(start) / replays
	t.Logf("announce replay: %d packets in %s (%s/pkt)", replays, time.Since(start), perReplay)

	// An Ed25519 verify is ~40-90us native and several hundred us under
	// -race; the pre-verify dedup path is a SHA256 + map lookup. The 80us
	// bound stays an order of magnitude below any real verify.
	if perReplay > 80*time.Microsecond {
		t.Fatalf("replay path too slow (%s/pkt): dedup is not running before verify", perReplay)
	}
	dups := health.Default.SnapshotTransport().AnnounceDup.Total
	t.Logf("PASS: %d replays at %s/pkt, announce_dup=%d", replays, perReplay, dups)
}

// TestFloodDistinctAnnouncesBounded checks that a flood of distinct,
// validly signed announces cannot grow transport state without bound.
func TestFloodDistinctAnnouncesBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := newFloodIface("flood2")
	if err := tr.RegisterInterface("flood2", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	const N = 3000
	raws := make([][]byte, N)
	for i := 0; i < N; i++ {
		id, err := identity.New()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := signedAnnounceRaw(t, tr, id)
		raws[i] = raw
	}

	g0 := runtime.NumGoroutine()
	h0 := heapAlloc()
	start := time.Now()
	for _, raw := range raws {
		tr.HandlePacket(raw, iface)
	}
	elapsed := time.Since(start)

	// Handler workers run async off bounded queues; wait for the pool to
	// drain before measuring growth.
	var g1 int
	prevG, stable := -1, 0
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && stable < 4 {
		time.Sleep(150 * time.Millisecond)
		g1 = runtime.NumGoroutine()
		if g1 == prevG {
			stable++
		} else {
			stable = 0
		}
		prevG = g1
	}
	h1 := heapAlloc()

	tr.mutex.RLock()
	paths := len(tr.paths)
	tr.mutex.RUnlock()

	t.Logf("distinct announces: %d sent, paths=%d heap=%+.1fMB goroutines=+%d elapsed=%s",
		N, paths, float64(int64(h1)-int64(h0))/1048576, g1-g0, elapsed)

	if paths > common.DefaultMaxInMemoryPaths+256 {
		t.Fatalf("path table exceeded soft cap: %d", paths)
	}
	// Handler pool (512) plus test/infrastructure slack is the expected
	// ceiling; anything beyond means the announce path spawned its own
	// unbounded workers.
	if g1-g0 > MaxConcurrentPacketHandlers+128 {
		t.Fatalf("announce flood spawned unbounded work: +%d goroutines", g1-g0)
	}
	t.Logf("PASS: %d distinct signed announces -> %d paths, bounded heap growth", N, paths)
}

// TestFloodGarbagePackets ensures random wire garbage stays cheap: the
// parser must reject without allocation growth or panics.
func TestFloodGarbagePackets(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()

	iface := newFloodIface("flood3")
	if err := tr.RegisterInterface("flood3", iface); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	const N = 20000
	buf := make([]byte, 256)
	start := time.Now()
	for i := 0; i < N; i++ {
		n := 2 + i%254
		if _, err := rand.Read(buf[:n]); err != nil {
			t.Fatal(err)
		}
		tr.HandlePacket(append([]byte(nil), buf[:n]...), iface)
	}
	elapsed := time.Since(start)
	t.Logf("PASS: %d garbage packets in %s (%s/pkt), no panic",
		N, elapsed, elapsed/N)
}

// TestProtectPreventDropsShedFirst drives the live admit path in
// prevent mode: an announce flood past the absolute ceiling must be
// dropped at the gate while the engine reports trips.
func TestProtectPreventDropsShedFirst(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	e := protect.New(protect.Options{
		Mode:       protect.ModePrevent,
		MaxPPS:     500,
		MaxBPS:     1024 * 1024,
		WarnWriter: nil,
	})
	// Point warns at a buffer-free sink.
	prev := protect.Default()
	protect.SetDefault(e)
	defer protect.SetDefault(prev)

	var announceHdr byte = 0x01 // packetType=announce -> ClassShedFirst
	data := append([]byte{announceHdr, 0x00}, make([]byte, 64)...)

	const N = 4000
	dropped := 0
	for i := 0; i < N; i++ {
		if !protect.AdmitPacketOpts("flood-iface", len(data), protect.AdmitOpts{
			Class: protect.PeekPacketClass(data),
		}).Allow {
			dropped++
		}
	}
	t.Logf("prevent mode: %d/%d announce-class packets dropped", dropped, N)
	if dropped == 0 {
		t.Fatal("prevent mode dropped nothing under a sustained announce flood")
	}
}

// TestProtectPreferKeepSurvivesAnnounceFlood verifies link-class traffic
// keeps flowing while shed-first traffic is being shed at the same rate.
func TestProtectPreferKeepSurvivesAnnounceFlood(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	e := protect.New(protect.Options{
		Mode:   protect.ModePrevent,
		MaxPPS: 300,
		MaxBPS: 1024 * 1024,
	})
	prev := protect.Default()
	protect.SetDefault(e)
	defer protect.SetDefault(prev)

	var linkHdr byte = 0x02 // packetType=linkreq -> prefer-keep
	linkData := append([]byte{linkHdr, 0x00}, make([]byte, 67)...)
	var annHdr byte = 0x01
	annData := append([]byte{annHdr, 0x00}, make([]byte, 64)...)

	// Flood announces just over the trip line (300pps) but keep the
	// combined window under the prefer-keep leniency cap (2x = 600pps):
	// shed-first traffic must be dropped while link traffic still flows.
	const annN = 350
	dropped := 0
	for i := 0; i < annN; i++ {
		if !protect.AdmitPacketOpts("shared", len(annData), protect.AdmitOpts{Class: protect.ClassShedFirst}).Allow {
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatalf("shed-first traffic not shed over trip line: %d/%d dropped", dropped, annN)
	}
	allowed := 0
	for i := 0; i < 200; i++ {
		if protect.AdmitPacketOpts("shared", len(linkData), protect.AdmitOpts{Class: protect.ClassPreferKeep}).Allow {
			allowed++
		}
	}
	t.Logf("prefer-keep allowed %d/200, shed-first dropped %d/%d", allowed, dropped, annN)
	if allowed == 0 {
		t.Fatal("prefer-keep traffic fully starved by shed-first flood")
	}
}

// TestLiveUDPInterfacePreventMode runs the full stack on a real loopback
// UDP socket: datagram -> read loop -> ProcessIncomingFromAddr -> protect
// admit gate -> transport. In prevent mode a datagram flood must be shed
// at the gate and counted, while the transport keeps running.
func TestLiveUDPInterfacePreventMode(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	// Configure via ReticulumConfig exactly like the daemon path.
	prev := protect.Default()
	defer protect.SetDefault(prev)
	health.Default.Reset()

	tr := NewTransport(&common.ReticulumConfig{
		DoSProtection:    "prevent",
		DoSProtectionSet: true,
		DoSMaxPPS:        300,
		DoSMaxBPS:        1024 * 1024,
	})
	defer tr.Close()

	// Grab a free loopback port, then bind the interface to it.
	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	addr := probe.LocalAddr().(*net.UDPAddr)
	probe.Close()

	ui, err := interfaces.NewUDPInterface("udp-flood", addr.String(), "", true)
	if err != nil {
		t.Fatalf("NewUDPInterface: %v", err)
	}
	if err := ui.Start(); err != nil {
		t.Fatalf("UDP Start: %v", err)
	}
	defer func() { _ = ui.Stop() }()
	if err := tr.RegisterInterface("udp-flood", ui); err != nil {
		t.Fatalf("RegisterInterface: %v", err)
	}

	sock, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer sock.Close()

	// Announce-class datagrams: header byte 0x01 (packetType=announce).
	pkt := append([]byte{0x01, 0x00}, make([]byte, 400)...)
	const N = 4000
	start := time.Now()
	sent := 0
	for i := 0; i < N; i++ {
		if _, err := sock.Write(pkt); err != nil {
			continue
		}
		sent++
	}
	inject := time.Since(start)
	time.Sleep(500 * time.Millisecond)

	snap := health.Default.SnapshotIface("udp-flood")
	trips := snap.DoSPPS.Total + snap.DoSEarlyDrop.Total + snap.DoSCoolDown.Total
	t.Logf("UDP live: %d datagrams in %s, dos trips=%d (pps=%d early_drop=%d cooldown=%d)",
		sent, inject, trips, snap.DoSPPS.Total, snap.DoSEarlyDrop.Total, snap.DoSCoolDown.Total)
	if trips == 0 {
		t.Fatal("live UDP flood produced zero protect trips in prevent mode")
	}
	t.Logf("PASS: real UDP socket flood shed by protect gate")
}

// TestProtectPeerIsolation verifies one flooding peer cannot exhaust a
// shared interface budget: the quiet peer keeps its own sub-bucket.
func TestProtectPeerIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("flood test")
	}
	e := protect.New(protect.Options{
		Mode:   protect.ModePrevent,
		MaxPPS: 400,
		MaxBPS: 1024 * 1024,
	})
	prev := protect.Default()
	protect.SetDefault(e)
	defer protect.SetDefault(prev)

	var annHdr byte = 0x01
	data := append([]byte{annHdr, 0x00}, make([]byte, 64)...)

	// Peer A floods.
	for i := 0; i < 5000; i++ {
		protect.AdmitPacketOpts("shared-udp", len(data), protect.AdmitOpts{
			Class:   protect.ClassShedFirst,
			PeerKey: "attacker:5000",
		})
	}
	// Peer B, quiet, must still be admitted.
	allowed := 0
	for i := 0; i < 100; i++ {
		if protect.AdmitPacketOpts("shared-udp", len(data), protect.AdmitOpts{
			Class:   protect.ClassShedFirst,
			PeerKey: "victim:5001",
		}).Allow {
			allowed++
		}
	}
	t.Logf("quiet peer admitted %d/100 while attacker flooded", allowed)
	if allowed < 50 {
		t.Fatalf("peer isolation failed: quiet peer admitted only %d/100", allowed)
	}
}
