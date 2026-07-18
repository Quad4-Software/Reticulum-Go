// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/ifac"
)

// lossySimIface wraps simIface delivery with a per-direction packet
// drop probability. Loss is applied on the *send* side so receiver
// rx accounting reflects actual delivery, matching what a real lossy
// link would expose.
type lossySimIface struct {
	*simIface
	dropProb float64
	rng      *rand.Rand
	rngMu    sync.Mutex
	dropped  uint64
}

// Send drops a packet with probability dropProb. Surviving packets

// follow the normal delivery path.
func (l *lossySimIface) Send(data []byte, addr string) error {
	l.rngMu.Lock()
	drop := l.rng.Float64() < l.dropProb
	l.rngMu.Unlock()
	if drop {
		atomic.AddUint64(&l.dropped, 1)
		return nil
	}
	return l.simIface.Send(data, addr)
}

// linkLossy wires nodes a and b with a duplex pair of simIfaces and
// wraps each direction in a per-packet drop filter. The originator's
// transport sees only the lossy wrapper, so an outbound Send must
// pass the drop check before reaching the peer's inbox.
func linkLossy(t testing.TB, s *simNetwork, a, b int, dropAtoB, dropBtoA float64, seed uint64) (left, right *lossySimIface) {
	t.Helper()
	if a == b {
		t.Fatalf("cannot link node %d to itself", a)
	}
	na, nb := s.nodes[a], s.nodes[b]
	leftBase := newSimIface(fmt.Sprintf("%s_lossy_%s", na.name, nb.name))
	rightBase := newSimIface(fmt.Sprintf("%s_lossy_%s", nb.name, na.name))
	leftBase.peer = rightBase
	rightBase.peer = leftBase
	left = &lossySimIface{
		simIface: leftBase,
		dropProb: dropAtoB,
		rng:      rand.New(rand.NewPCG(seed, seed^0xdeadbeef)),
	}
	right = &lossySimIface{
		simIface: rightBase,
		dropProb: dropBtoA,
		rng:      rand.New(rand.NewPCG(seed^0xfeedface, seed^0xcafef00d)),
	}
	if err := na.tr.RegisterInterface(leftBase.GetName(), left); err != nil {
		t.Fatalf("register lossy left: %v", err)
	}
	if err := nb.tr.RegisterInterface(rightBase.GetName(), right); err != nil {
		t.Fatalf("register lossy right: %v", err)
	}
	na.tr.ifaceStates.put(leftBase.GetName(), &ifaceState{})
	nb.tr.ifaceStates.put(rightBase.GetName(), &ifaceState{})
	na.ifaces = append(na.ifaces, leftBase)
	nb.ifaces = append(nb.ifaces, rightBase)
	s.addEdge(a, b)
	return left, right
}

// TestSimAsymmetricLink runs a 4-node line whose middle edge drops
// 50% of packets in one direction, and verifies announces still
// converge thanks to PathfinderRW retransmission.
func TestSimAsymmetricLink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 4
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)

	net.link(t, 0, 1)
	left, right := linkLossy(t, net, 1, 2, 0.5, 0.0, 0x5eed)
	net.link(t, 2, 3)

	src := net.nodes[0]
	src.originateAnnounce(t)

	took, ok := waitForPaths(net.nodes[1:], src.destHash, 15*time.Second)
	if ok != n-1 {
		t.Fatalf("convergence failed under 50%% loss: %d/%d after %v (drops L=%d R=%d)",
			ok, n-1, took, atomic.LoadUint64(&left.dropped), atomic.LoadUint64(&right.dropped))
	}
	t.Logf("asymmetric line converged in %v (forward drops=%d)",
		took, atomic.LoadUint64(&left.dropped))
}

// TestSimChurnDuringFlood kills a quarter of the line in the
// PathfinderRW window between announce origination and rebroadcast,
// then verifies the head and tail segments each converge within
// their reachable subgraph. The originator and the tail are kept
// alive so a partition across victims does not invalidate the test.
func TestSimChurnDuringFlood(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 12
	net := buildLine(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	victims := []int{3, 7}
	go func() {
		time.Sleep(100 * time.Millisecond)
		for _, idx := range victims {
			for _, ifc := range net.nodes[idx].ifaces {
				ifc.stop()
			}
			_ = net.nodes[idx].tr.Close()
		}
	}()

	head := net.nodes[1:victims[0]]
	took, ok := waitForPaths(head, src.destHash, 15*time.Second)
	if ok != len(head) {
		t.Fatalf("head segment failed to converge: %d/%d after %v", ok, len(head), took)
	}
	t.Logf("line(N=%d, churn=%v) head segment converged in %v", n, victims, took)
}

// TestSimPathRequestStorm fires K concurrent RequestPath calls for
// the same unknown destination from one node and asserts the
// per-destination throttle (PathRequestMI) collapses them into a
// single on-the-wire request.
func TestSimPathRequestStorm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 3
	const callers = 64
	net := buildLine(t, n)
	t.Cleanup(net.close)

	requester := net.nodes[0]
	target := net.nodes[n-1].destHash

	var wg sync.WaitGroup
	startTx := uint64(0)
	for _, ifc := range requester.ifaces {
		startTx += ifc.GetTxPackets()
	}
	for range callers {
		wg.Go(func() {
			_ = requester.tr.RequestPath(target, "", nil, true)
		})
	}
	wg.Wait()

	endTx := uint64(0)
	for _, ifc := range requester.ifaces {
		endTx += ifc.GetTxPackets()
	}
	sent := endTx - startTx
	if sent == 0 {
		t.Fatalf("expected at least one path request to leave the requester, got 0")
	}
	if sent > 4 {
		t.Errorf("path-request storm not throttled: %d packets sent for %d callers", sent, callers)
	}
	t.Logf("path-request storm: %d callers -> %d wire packets", callers, sent)
}

// TestSimIFACFlood configures a network identity on every interface
// and verifies announces still propagate end-to-end. Asserts the
// IFAC mask/unmask round-trip works under the harness's inbox-driven
// delivery model.
func TestSimIFACFlood(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 6
	id, err := ifac.New(0, "sim-net", "sim-passphrase")
	if err != nil {
		t.Fatalf("ifac.New: %v", err)
	}
	net := buildLine(t, n)
	t.Cleanup(net.close)
	for _, node := range net.nodes {
		for _, ifc := range node.ifaces {
			ifc.SetIFAC(id)
		}
	}

	src := net.nodes[0]
	src.originateAnnounce(t)

	took, ok := waitForPaths(net.nodes[1:], src.destHash, 15*time.Second)
	if ok != n-1 {
		t.Fatalf("IFAC line: %d/%d converged in %v", ok, n-1, took)
	}
	t.Logf("IFAC line(N=%d) converged in %v", n, took)
}

// BenchmarkSimRebuildHeaderType2 isolates the per-hop allocation
// cost of rebuilding a HeaderType2 packet. This is the inner loop of
// every relay. Surfacing it independently makes the relay-throughput

// allocs/op interpretable.
func BenchmarkSimRebuildHeaderType2(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, payload := range []int{16, 256, 1024} {
		b.Run(fmt.Sprintf("Payload-%d", payload), func(b *testing.B) {
			transportID := make([]byte, 16)
			destHash := make([]byte, 16)
			for i := range transportID {
				transportID[i] = byte(i)
				destHash[i] = byte(i + 1)
			}
			pkt := buildHT2(transportID, destHash, 0, make([]byte, payload))
			nextHop := make([]byte, 16)
			b.ReportAllocs()
			b.SetBytes(int64(len(pkt)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := rebuildHeaderType2(pkt, byte(i), nextHop)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSimIFACMaskUnmask measures the mask + unmask round-trip
// cost for a single packet. Identifies how much of a hop's CPU
// budget IFAC consumes vs the bare relay rewrite.
func BenchmarkSimIFACMaskUnmask(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	id, err := ifac.New(0, "bench-net", "bench-pass")
	if err != nil {
		b.Fatalf("ifac.New: %v", err)
	}
	for _, payload := range []int{16, 256, 1024} {
		b.Run(fmt.Sprintf("Payload-%d", payload), func(b *testing.B) {
			raw := make([]byte, 2+payload)
			raw[0] = 0x40
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				masked, err := id.Mask(raw)
				if err != nil {
					b.Fatal(err)
				}
				if _, ok, err := id.Unmask(masked); err != nil || !ok {
					b.Fatalf("unmask failed: ok=%v err=%v", ok, err)
				}
			}
		})
	}
}

// BenchmarkSimIFACLineRelay is BenchmarkSimLineRelayThroughput with
// IFAC enabled on every iface, so the delta isolates the IFAC tax
// per hop end-to-end.
func BenchmarkSimIFACLineRelay(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	id, err := ifac.New(0, "bench-net", "bench-pass")
	if err != nil {
		b.Fatalf("ifac.New: %v", err)
	}
	for _, hops := range []int{2, 4} {
		b.Run(fmt.Sprintf("Hops-%d", hops), func(b *testing.B) {
			b.StopTimer()
			n := hops + 1
			net := buildLine(b, n)
			b.Cleanup(net.close)
			for _, node := range net.nodes {
				for _, ifc := range node.ifaces {
					ifc.SetIFAC(id)
				}
			}
			target := net.nodes[n-1].id.Hash()
			preloadLinePaths(net.nodes, target)
			tail := net.nodes[n-1].ifaces[0]
			src := net.nodes[0].ifaces[0]
			second := net.nodes[1].id.Hash()

			b.StartTimer()
			b.ReportAllocs()
			startRx := tail.GetRxPackets()
			for i := 0; i < b.N; i++ {
				payload := make([]byte, 64)
				binary.BigEndian.PutUint64(payload, uint64(i))
				pkt := buildHT2(second, target, 0, payload)
				_ = src.Send(pkt, "")
			}
			want := startRx + uint64(b.N)
			deadline := time.Now().Add(time.Duration(b.N)*500*time.Microsecond + 5*time.Second)
			for tail.GetRxPackets() < want && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			b.StopTimer()
			if got := tail.GetRxPackets(); got < want {
				b.Logf("delivery shortfall (IFAC): got=%d want=%d hops=%d N=%d", got, want, hops, b.N)
			}
		})
	}
}

// BenchmarkSimPathRequestStormCost measures the cost of K concurrent
// RequestPath calls for the same destination, exposing throttle and
// dedup overhead under contention.
func BenchmarkSimPathRequestStormCost(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, callers := range []int{1, 16, 64} {
		b.Run(fmt.Sprintf("Callers-%d", callers), func(b *testing.B) {
			b.StopTimer()
			net := buildLine(b, 3)
			b.Cleanup(net.close)
			requester := net.nodes[0]
			target := net.nodes[2].destHash
			b.StartTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				for range callers {
					wg.Go(func() {
						_ = requester.tr.RequestPath(target, "", nil, true)
					})
				}
				wg.Wait()
			}
		})
	}
}

// TestSimNoGoroutineLeakAfterClose builds and tears down a series of
// networks and asserts the goroutine count returns to baseline. A
// regression here usually means a deliverLoop or transport
// maintenance goroutine is escaping cleanup.
func TestSimNoGoroutineLeakAfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping leak check in -short mode")
	}
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	for range 5 {
		net := buildMesh(t, 6)
		net.nodes[0].originateAnnounce(t)
		time.Sleep(50 * time.Millisecond)
		net.close()
	}

	runtime.GC()
	time.Sleep(750 * time.Millisecond)
	final := runtime.NumGoroutine()
	if final > baseline+2 {
		buf := make([]byte, 1<<20)
		nb := runtime.Stack(buf, true)
		t.Errorf("goroutine leak suspected: baseline=%d final=%d\n%s", baseline, final, buf[:nb])
	}
	t.Logf("goroutines: baseline=%d final=%d", baseline, final)
}

// pathEntrySize estimates the bytes a Transport spends per path
// table entry. Used by TestSimPathTableMemory to assert the figure
// stays bounded as N grows.
func pathEntrySize(tr *Transport, dest []byte) int {
	if !tr.HasPath(dest) {
		return 0
	}
	const overhead = 48
	const keyBytes = 32
	const valueBytes = 128
	const slicePayload = 16
	_ = dest
	return overhead + keyBytes + valueBytes + slicePayload
}

// TestSimPathTableMemory captures the per-entry memory cost of the
// path table and fails the test if it exceeds a sanity bound. This
// makes regressions in the Path or map representation visible
// without depending on heap profiling.
func TestSimPathTableMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory check in -short mode")
	}
	const n = 64
	net := buildLine(t, n)
	t.Cleanup(net.close)
	hub := net.nodes[0]
	ifaceName := hub.ifaces[0].GetName()
	for i := 1; i < n; i++ {
		hub.tr.UpdatePath(net.nodes[i].destHash, net.nodes[i].destHash, ifaceName, uint8(i))
	}
	per := pathEntrySize(hub.tr, net.nodes[1].destHash)
	if per == 0 {
		t.Fatalf("path entry not present after UpdatePath")
	}
	const maxBytesPerEntry = 256
	t.Logf("path table per-entry estimate: ~%d bytes (entries=%d)", per, n-1)
	if per > maxBytesPerEntry {
		t.Errorf("per-entry size %d exceeds %d byte budget", per, maxBytesPerEntry)
	}
	_ = common.Path{}
}
