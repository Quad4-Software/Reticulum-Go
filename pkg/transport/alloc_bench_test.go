// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/sha256"
	"fmt"
	"runtime"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/announce"
	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/debug"
	"quad4/reticulum-go/pkg/identity"
)

// buildValidAnnounceForReceiver produces a valid announce wire payload that
// passes destination-hash validation on a freshly built transport whose
// identity is the announcing identity. Returns the raw announce bytes.
func buildValidAnnounceForReceiver(t testing.TB) ([]byte, *Transport, *simIface) {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	tr.SetIdentity(id)

	destName := "bench.node.0"
	nameHash := sha256.Sum256([]byte(destName))
	destInput := append(nameHash[:10], id.Hash()...)
	destFull := sha256.Sum256(destInput)
	destHash := destFull[:16]

	a, err := announce.New(id, destHash, destName, []byte(destName), false, &common.ReticulumConfig{})
	if err != nil {
		t.Fatalf("build announce: %v", err)
	}
	pkt, err := a.CreatePacket()
	if err != nil {
		t.Fatalf("create packet: %v", err)
	}

	iface := newSimIface("bench0")
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register iface: %v", err)
	}
	tr.ifaceStates.put(iface.GetName(), &ifaceState{})
	return pkt, tr, iface
}

// benchHandleAnnounce drives the full handleAnnouncePacket path (parse,
// verify, path update, forward fan-out). enableSimFastPath zeroes the
// pathfinder rebroadcast delay so the bench is not dominated by sleeps.
func benchHandleAnnounce(b *testing.B, level int) {
	enableSimFastPath(b)
	prev := debug.GetDebugLevel()
	debug.SetDebugLevel(level)
	b.Logf("effective debug level: %d (requested %d)", debug.GetDebugLevel(), level)
	b.Cleanup(func() { debug.SetDebugLevel(prev) })

	pkt, tr, iface := buildValidAnnounceForReceiver(b)
	defer tr.Close()
	defer iface.stop()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.mutex.Lock()
		clear(tr.seenAnnounces)
		tr.mutex.Unlock()
		_ = tr.handleAnnouncePacket(pkt, iface)
	}
}

func BenchmarkHandleAnnouncePacket_DebugCritical(b *testing.B) {
	benchHandleAnnounce(b, debug.DebugCritical)
}

func BenchmarkHandleAnnouncePacket_DebugInfo(b *testing.B) {
	benchHandleAnnounce(b, debug.DebugInfo)
}

func BenchmarkHandleAnnouncePacket_DebugVerbose(b *testing.B) {
	benchHandleAnnounce(b, debug.DebugVerbose)
}

// BenchmarkHandlePacketCopy isolates the per-packet defensive byte copy in
// Transport.HandlePacket (make+copy for async goroutine dispatch).
func BenchmarkHandlePacketCopy(b *testing.B) {
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cp := make([]byte, len(data))
		copy(cp, data)
		_ = cp
	}
}

// BenchmarkSnapshotRegisteredInterfaces measures the cost of building a
// shallow interface snapshot, which happens on every announce forward and
// path request fan-out.
func BenchmarkSnapshotRegisteredInterfaces(b *testing.B) {
	tr := NewTransport(&common.ReticulumConfig{})
	defer tr.Close()
	for i := range 8 {
		iface := newSimIface(fmt.Sprintf("i%d", i))
		if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
			b.Fatalf("register: %v", err)
		}
		defer iface.stop()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tr.snapshotRegisteredInterfaces()
	}
}

// BenchmarkDebugLogCallSite_Filtered measures the cost of a debug.Log call
// whose arguments are evaluated at the call site but whose level is filtered
// out by the runtime threshold. This mirrors the dominant pattern in the
// transport hot path:
//
//	debug.Log(debug.DebugVerbose, "msg", "hash", fmt.Sprintf("%x", h))
//
// Every fmt.Sprintf and the variadic []any slice are allocated before Log
// discards them, which is the hidden allocator pressure on a busy network
// running at the default info level.
func BenchmarkDebugLogCallSite_Filtered(b *testing.B) {
	hash := sha256.Sum256([]byte("deadbeef"))
	filteredLevel := debug.DebugVerbose
	debug.SetDebugLevel(debug.DebugInfo) // verbose is filtered here
	b.Cleanup(func() { debug.SetDebugLevel(debug.DebugInfo) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		debug.Log(filteredLevel, "Transport handling announce",
			"bytes", 211, "hash", fmt.Sprintf("%x", hash[:16]))
	}
}

func BenchmarkDebugLogCallSite_NoArgs(b *testing.B) {
	filteredLevel := debug.DebugVerbose
	debug.SetDebugLevel(debug.DebugInfo)
	b.Cleanup(func() { debug.SetDebugLevel(debug.DebugInfo) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		debug.Log(filteredLevel, "Processing announce packet")
	}
}

// TestAnnounceIngestNoGrowth runs a large burst of announce ingests and
// asserts that reachable heap after GC does not grow unboundedly across
// batches. A leak would show monotonic growth between batches.
func TestAnnounceIngestNoGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping leak-growth test in short mode")
	}
	enableSimFastPath(t)
	debug.SetDebugLevel(debug.DebugCritical)

	pkt, tr, iface := buildValidAnnounceForReceiver(t)
	defer tr.Close()
	defer iface.stop()

	const batches = 4
	const perBatch = 50_000
	var samples [batches]uint64

	for range perBatch {
		tr.mutex.Lock()
		clear(tr.seenAnnounces)
		tr.mutex.Unlock()
		_ = tr.handleAnnouncePacket(pkt, iface)
	}
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	samples[0] = m.HeapAlloc

	for b := 1; b < batches; b++ {
		for range perBatch {
			tr.mutex.Lock()
			clear(tr.seenAnnounces)
			tr.mutex.Unlock()
			_ = tr.handleAnnouncePacket(pkt, iface)
		}
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&m)
		samples[b] = m.HeapAlloc
	}

	t.Logf("HeapAlloc by batch (after GC): %v bytes", samples)
	if samples[batches-1] > samples[0]+(2<<20) && samples[batches-1] > samples[1]+(1<<20) {
		t.Errorf("possible leak: HeapAlloc grew %d -> %d bytes across batches",
			samples[0], samples[batches-1])
	}
	time.Sleep(200 * time.Millisecond)
	if g := runtime.NumGoroutine(); g > 5 {
		t.Logf("goroutines after run: %d (maintenance goroutine expected)", g)
	}
}
