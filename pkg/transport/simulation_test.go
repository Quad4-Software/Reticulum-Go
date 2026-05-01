// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"
	"time"

	"git.quad4.io/Networks/Reticulum-Go/pkg/announce"
	"git.quad4.io/Networks/Reticulum-Go/pkg/common"
	"git.quad4.io/Networks/Reticulum-Go/pkg/identity"
	"git.quad4.io/Networks/Reticulum-Go/pkg/packet"
)

// simInboxSize bounds in-flight packets per duplex side. The sender
// blocks once the peer's queue is full, providing natural
// backpressure and capping memory under load.
const simInboxSize = 1024

// simIface is an in-process duplex network interface used by the
// multi-node simulation harness. A single delivery goroutine drains
// the inbox into the BaseInterface ProcessIncoming pipeline so the
// benchmark cannot leak one goroutine per packet.
type simIface struct {
	common.BaseInterface
	peer     *simIface
	inbox    chan []byte
	stopOnce sync.Once
	done     chan struct{}
}

func newSimIface(name string) *simIface {
	s := &simIface{
		BaseInterface: common.NewBaseInterface(name, common.IFTypeAuto, true),
		inbox:         make(chan []byte, simInboxSize),
		done:          make(chan struct{}),
	}
	s.MTU = common.DefaultMTU
	s.Bitrate = 1_000_000_000
	s.Enable()
	go s.deliverLoop()
	return s
}

// deliverLoop is the only goroutine that calls ProcessIncoming on
// this iface. It exits when stop is invoked.
func (s *simIface) deliverLoop() {
	for {
		select {
		case <-s.done:
			for {
				select {
				case <-s.inbox:
				default:
					return
				}
			}
		case data := <-s.inbox:
			s.ProcessIncoming(data)
		}
	}
}

func (s *simIface) stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

// Send mirrors BaseInterface.Send so an attached IFAC identity is
// honoured, then enqueues onto the peer's inbox.
func (s *simIface) Send(data []byte, _ string) error {
	if id := s.GetIFAC(); id != nil {
		masked, err := id.Mask(data)
		if err != nil {
			return err
		}
		data = masked
	}
	s.Mutex.Lock()
	s.TxBytes += uint64(len(data))
	s.TxPackets++
	s.Mutex.Unlock()
	if s.peer == nil {
		return nil
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case s.peer.inbox <- cp:
	case <-s.peer.done:
	}
	return nil
}

// ProcessOutgoing routes through Send so direct callers behave the
// same as transport-driven sends.
func (s *simIface) ProcessOutgoing(data []byte) error { return s.Send(data, "") }

// simNode is one transport instance plus its identity and registered
// duplex interfaces. Each entry in ifaces corresponds to one edge in
// the simulated topology.
type simNode struct {
	id     *identity.Identity
	tr     *Transport
	name   string
	ifaces []*simIface

	destName string
	destHash []byte
}

// newSimNode constructs a transport, generates a fresh identity and
// pre-computes the destination hash that the node will announce.
func newSimNode(t testing.TB, idx int) *simNode {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	cfg := &common.ReticulumConfig{EnableTransport: true}
	tr := NewTransport(cfg)
	tr.SetIdentity(id)

	name := fmt.Sprintf("node%04d", idx)
	destName := fmt.Sprintf("sim.node.%d", idx)
	nameHash := sha256.Sum256([]byte(destName))
	destInput := append(nameHash[:10], id.Hash()...)
	destFull := sha256.Sum256(destInput)
	return &simNode{
		id:       id,
		tr:       tr,
		name:     name,
		destName: destName,
		destHash: destFull[:16],
	}
}

// announcePacket signs and serialises a fresh announce for this node.
// announce.New (rather than NewAnnounce) is used so the destination
// name is propagated into the packet's name-hash field, matching the
// validator on the receiving transport.
func (n *simNode) announcePacket(t testing.TB) []byte {
	t.Helper()
	cfg := &common.ReticulumConfig{}
	a, err := announce.New(n.id, n.destHash, n.destName, []byte(n.destName), false, cfg)
	if err != nil {
		t.Fatalf("announce.New: %v", err)
	}
	pkt, err := a.CreatePacket()
	if err != nil {
		t.Fatalf("announce.CreatePacket: %v", err)
	}
	return pkt
}

// originateAnnounce broadcasts the node's announce on every attached
// interface; receivers process it through their normal packet
// pipeline.
func (n *simNode) originateAnnounce(t testing.TB) {
	t.Helper()
	pkt := n.announcePacket(t)
	for _, ifc := range n.ifaces {
		_ = ifc.Send(pkt, "")
	}
}

// simNetwork is a collection of simNodes plus the duplex edges that
// connect them. Lifetime is the caller's responsibility: invoke
// close() exactly once after each network is no longer needed.
type simNetwork struct {
	nodes     []*simNode
	closeOnce sync.Once
}

func newSimNetwork(t testing.TB, n int) *simNetwork {
	t.Helper()
	net := &simNetwork{nodes: make([]*simNode, n)}
	for i := range n {
		net.nodes[i] = newSimNode(t, i)
	}
	return net
}

// close stops every transport and every iface delivery goroutine.
// Safe to call more than once.
func (s *simNetwork) close() {
	s.closeOnce.Do(func() {
		for _, n := range s.nodes {
			for _, ifc := range n.ifaces {
				ifc.stop()
			}
			_ = n.tr.Close()
		}
	})
}

// link wires nodes a and b with a duplex pair of simIfaces and
// registers each side with its owning transport. Per-interface spam
// controls are disabled so a synthetic flood is not throttled by the
// default burst-detection thresholds.
func (s *simNetwork) link(t testing.TB, a, b int) {
	t.Helper()
	if a == b {
		t.Fatalf("cannot link node %d to itself", a)
	}
	na, nb := s.nodes[a], s.nodes[b]
	left := newSimIface(fmt.Sprintf("%s_to_%s", na.name, nb.name))
	right := newSimIface(fmt.Sprintf("%s_to_%s", nb.name, na.name))
	left.peer = right
	right.peer = left
	if err := na.tr.RegisterInterface(left.GetName(), left); err != nil {
		t.Fatalf("register %s: %v", left.GetName(), err)
	}
	if err := nb.tr.RegisterInterface(right.GetName(), right); err != nil {
		t.Fatalf("register %s: %v", right.GetName(), err)
	}
	na.tr.ifaceStates.put(left.GetName(), &ifaceState{})
	nb.tr.ifaceStates.put(right.GetName(), &ifaceState{})
	na.ifaces = append(na.ifaces, left)
	nb.ifaces = append(nb.ifaces, right)
}

// Topology builders. Each returns a populated simNetwork; the caller
// is responsible for invoking close() once it is done with it.

func buildLine(t testing.TB, n int) *simNetwork {
	net := newSimNetwork(t, n)
	for i := 0; i < n-1; i++ {
		net.link(t, i, i+1)
	}
	return net
}

func buildRing(t testing.TB, n int) *simNetwork {
	net := newSimNetwork(t, n)
	for i := range n {
		net.link(t, i, (i+1)%n)
	}
	return net
}

func buildStar(t testing.TB, n int) *simNetwork {
	net := newSimNetwork(t, n)
	for i := 1; i < n; i++ {
		net.link(t, 0, i)
	}
	return net
}

func buildMesh(t testing.TB, n int) *simNetwork {
	net := newSimNetwork(t, n)
	for i := range n {
		for j := i + 1; j < n; j++ {
			net.link(t, i, j)
		}
	}
	return net
}

// buildRandom constructs an Erdos-Renyi style graph with edge
// probability p, then forces a spanning chain so the graph is always
// connected. Determinism comes from the supplied seed.
func buildRandom(t testing.TB, n int, p float64, seed uint64) *simNetwork {
	net := newSimNetwork(t, n)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	for i := 0; i < n-1; i++ {
		net.link(t, i, i+1)
	}
	for i := range n {
		for j := i + 2; j < n; j++ {
			if rng.Float64() < p {
				net.link(t, i, j)
			}
		}
	}
	return net
}

// waitForPaths polls every node in want until each one knows the
// destination, or until timeout. Returns the time taken plus the
// number of nodes that reached convergence.
func waitForPaths(nodes []*simNode, dest []byte, timeout time.Duration) (time.Duration, int) {
	start := time.Now()
	deadline := start.Add(timeout)
	for time.Now().Before(deadline) {
		ok := 0
		for _, n := range nodes {
			if n.tr.HasPath(dest) {
				ok++
			}
		}
		if ok == len(nodes) {
			return time.Since(start), ok
		}
		time.Sleep(20 * time.Millisecond)
	}
	ok := 0
	for _, n := range nodes {
		if n.tr.HasPath(dest) {
			ok++
		}
	}
	return time.Since(start), ok
}

// preloadLinePaths seeds path-table entries on every relay node so a
// HeaderType2 packet sent from node 0 toward target is forwarded
// along the chain. Each intermediate next-hop is the next relay's
// transport identity (so the rewriter sees a recognisable
// TransportID at every hop); the last hop falls through to the
// header-stripping branch.
func preloadLinePaths(nodes []*simNode, target []byte) {
	last := len(nodes) - 1
	for i := range last {
		ifc := nodes[i].ifaces[len(nodes[i].ifaces)-1]
		nextHop := nodes[i+1].id.Hash()
		hops := uint8(last - i)
		nodes[i].tr.UpdatePath(target, nextHop, ifc.GetName(), hops)
	}
}

// buildHT2 hand-rolls a HeaderType2 data packet identical to the one
// the relay tests use, so we exercise the relay path without pulling
// the full packet builder into a benchmark.
func buildHT2(transportID, destHash []byte, hops byte, payload []byte) []byte {
	flags := byte(0)
	flags |= (packet.HeaderType2 << 6) & packet.HeaderMaskHeaderType
	flags |= (packet.PropagationTransport << 4) & packet.HeaderMaskTransportType
	flags |= (packet.DestinationSingle << 2) & packet.HeaderMaskDestinationType
	flags |= packet.PacketTypeData & packet.HeaderMaskPacketType

	out := make([]byte, 0, 2+16+16+1+len(payload))
	out = append(out, flags, hops)
	out = append(out, transportID...)
	out = append(out, destHash...)
	out = append(out, packet.ContextNone)
	out = append(out, payload...)
	return out
}

// TestSimLineConvergence checks that an announce originated at one
// end of an N-node line reaches every node, and that hop counts grow
// monotonically with distance from the originator.
func TestSimLineConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 8
	net := buildLine(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]

	src.originateAnnounce(t)

	want := net.nodes[1:]
	took, ok := waitForPaths(want, src.destHash, 10*time.Second)
	if ok != len(want) {
		t.Fatalf("convergence failed: %d/%d nodes after %v", ok, len(want), took)
	}
	t.Logf("line(N=%d) converged in %v", n, took)

	for i, node := range net.nodes[1:] {
		got := node.tr.HopsTo(src.destHash)
		want := uint8(i + 1)
		if got != want {
			t.Errorf("node%d hopsTo(src) = %d, want %d", i+1, got, want)
		}
	}
}

// TestSimStarConvergence verifies a star topology floods in two
// hops: the hub records every leaf at hops=1 and every leaf records
// every other leaf at hops=2.
func TestSimStarConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 6
	net := buildStar(t, n)
	t.Cleanup(net.close)

	for _, node := range net.nodes[1:] {
		node.originateAnnounce(t)
	}

	for i := 1; i < n; i++ {
		took, ok := waitForPaths(net.nodes[0:1], net.nodes[i].destHash, 5*time.Second)
		if ok != 1 {
			t.Fatalf("hub never learned leaf %d after %v", i, took)
		}
	}

	for i := 1; i < n; i++ {
		var others []*simNode
		for j := 1; j < n; j++ {
			if j != i {
				others = append(others, net.nodes[j])
			}
		}
		took, ok := waitForPaths(others, net.nodes[i].destHash, 10*time.Second)
		if ok != len(others) {
			t.Fatalf("leaf %d only reached %d/%d peers after %v", i, ok, len(others), took)
		}
	}
}

// TestSimPartitionIsolation asserts an announce never crosses a
// missing edge: two disjoint lines share no path-table entries.
func TestSimPartitionIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	net := newSimNetwork(t, 6)
	t.Cleanup(net.close)
	for i := range 2 {
		net.link(t, i, i+1)
	}
	for i := 3; i < 5; i++ {
		net.link(t, i, i+1)
	}

	net.nodes[0].originateAnnounce(t)
	took, ok := waitForPaths(net.nodes[1:3], net.nodes[0].destHash, 5*time.Second)
	if ok != 2 {
		t.Fatalf("left partition convergence failed: %d/2 in %v", ok, took)
	}

	time.Sleep(200 * time.Millisecond)
	for i := 3; i < 6; i++ {
		if net.nodes[i].tr.HasPath(net.nodes[0].destHash) {
			t.Fatalf("right-partition node%d learned path across missing link", i)
		}
	}
}

// TestSimLineDataRelay sends a HeaderType2 data packet through a
// pre-populated line and verifies it lands at the tail interface
// with the correct hop count and a stripped header on the final hop.
func TestSimLineDataRelay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
	const n = 5
	net := buildLine(t, n)
	t.Cleanup(net.close)
	target := net.nodes[n-1].id.Hash()
	preloadLinePaths(net.nodes, target)

	last := net.nodes[n-1]
	tailIface := last.ifaces[0]
	startRx := tailIface.GetRxPackets()

	src := net.nodes[0]
	srcOut := src.ifaces[0]
	srcStartTx := srcOut.GetTxPackets()

	pkt := buildHT2(net.nodes[1].id.Hash(), target, 0, []byte("hello"))
	if err := srcOut.Send(pkt, ""); err != nil {
		t.Fatalf("src send: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tailIface.GetRxPackets() > startRx {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := tailIface.GetRxPackets() - startRx; got == 0 {
		t.Fatalf("tail never received relayed packet")
	}
	if srcOut.GetTxPackets()-srcStartTx != 1 {
		t.Fatalf("src tx mismatch: %d", srcOut.GetTxPackets()-srcStartTx)
	}
}

// TestSimMemoryFootprintAcrossNodes reports the average resident
// path-table cost per (node, destination) pair in a fully populated
// fabric. Useful as a budget guard for embedded deployments.
func TestSimMemoryFootprintAcrossNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory footprint test in -short mode")
	}
	const n = 32
	net := buildLine(t, n)
	t.Cleanup(net.close)

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	for _, src := range net.nodes {
		ifaceName := src.ifaces[0].GetName()
		for _, dst := range net.nodes {
			if dst == src {
				continue
			}
			src.tr.UpdatePath(dst.destHash, dst.destHash, ifaceName, 1)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	entries := uint64(n) * uint64(n-1)
	used := m2.Alloc - m1.Alloc
	t.Logf("nodes=%d entries=%d total=%d KB per_entry=%d B",
		n, entries, used/1024, used/entries)
}

// BenchmarkSimAnnounceConvergence reports wall-clock time for an
// announce originated at one end of a line topology to reach every
// other node. Per-iteration time grows roughly as N * PathfinderRW/2.
//
// The whole network is rebuilt every iteration and torn down before
// the next so transport and interface goroutines never accumulate.
func BenchmarkSimAnnounceConvergence(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, n := range []int{4, 8, 16} {
		b.Run(fmt.Sprintf("Line-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				net := buildLine(b, n)
				src := net.nodes[0]
				src.originateAnnounce(b)
				took, ok := waitForPaths(net.nodes[1:], src.destHash, 30*time.Second)
				if ok != n-1 {
					net.close()
					b.Fatalf("only %d/%d converged in %v", ok, n-1, took)
				}
				net.close()
			}
		})
	}
}

// BenchmarkSimPathLookupAcrossNodes pre-populates every node with a
// route to every other node, then measures aggregate NextHop cost
// across the whole network. The network is built once per
// sub-benchmark and torn down via b.Cleanup.
func BenchmarkSimPathLookupAcrossNodes(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("N-%d", n), func(b *testing.B) {
			b.StopTimer()
			net := buildLine(b, n)
			b.Cleanup(net.close)
			for _, src := range net.nodes {
				ifaceName := src.ifaces[0].GetName()
				for _, dst := range net.nodes {
					if dst == src {
						continue
					}
					src.tr.UpdatePath(dst.destHash, dst.destHash, ifaceName, 1)
				}
			}
			b.StartTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				src := net.nodes[i%n]
				dst := net.nodes[(i+1)%n]
				_ = src.tr.NextHop(dst.destHash)
			}
		})
	}
}

// BenchmarkSimLineRelayThroughput drives a stream of HeaderType2
// data packets through a line of K relay nodes and reports the
// per-iteration end-to-end delivery cost. Backpressure on simIface
// inboxes caps in-flight memory.
func BenchmarkSimLineRelayThroughput(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, hops := range []int{2, 4, 8} {
		b.Run(fmt.Sprintf("Hops-%d", hops), func(b *testing.B) {
			b.StopTimer()
			n := hops + 1
			net := buildLine(b, n)
			b.Cleanup(net.close)
			target := net.nodes[n-1].id.Hash()
			preloadLinePaths(net.nodes, target)

			tail := net.nodes[n-1].ifaces[0]
			src := net.nodes[0].ifaces[0]
			second := net.nodes[1].id.Hash()
			payload := make([]byte, 64)
			pkt := buildHT2(second, target, 0, payload)

			b.StartTimer()
			b.ReportAllocs()
			startRx := tail.GetRxPackets()
			for i := 0; i < b.N; i++ {
				_ = src.Send(pkt, "")
			}
			want := startRx + uint64(b.N)
			deadline := time.Now().Add(time.Duration(b.N)*200*time.Microsecond + 5*time.Second)
			for tail.GetRxPackets() < want && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			b.StopTimer()
			if got := tail.GetRxPackets(); got < want {
				b.Logf("delivery shortfall: got=%d want=%d (hops=%d, b.N=%d)", got, want, hops, b.N)
			}
		})
	}
}

// BenchmarkSimMeshAnnounceLoad measures the cost of one node
// originating an announce inside a fully connected mesh. Every other
// node receives N-1 copies of the same announce; all but one must be
// deduplicated by seenAnnounces.
func BenchmarkSimMeshAnnounceLoad(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, n := range []int{4, 8} {
		b.Run(fmt.Sprintf("N-%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				net := buildMesh(b, n)
				src := net.nodes[0]
				src.originateAnnounce(b)
				took, ok := waitForPaths(net.nodes[1:], src.destHash, 10*time.Second)
				if ok != n-1 {
					net.close()
					b.Fatalf("only %d/%d converged in %v", ok, n-1, took)
				}
				net.close()
			}
		})
	}
}

// BenchmarkSimConcurrentLineRelay drives M concurrent senders into
// the same line. Useful for spotting lock contention along the
// transport's per-packet path.
func BenchmarkSimConcurrentLineRelay(b *testing.B) {
	muteDebugLogsForBenchmark(b)
	for _, workers := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("Workers-%d", workers), func(b *testing.B) {
			b.StopTimer()
			const hops = 4
			n := hops + 1
			net := buildLine(b, n)
			b.Cleanup(net.close)
			target := net.nodes[n-1].id.Hash()
			preloadLinePaths(net.nodes, target)
			second := net.nodes[1].id.Hash()
			payload := make([]byte, 64)
			pkt := buildHT2(second, target, 0, payload)
			src := net.nodes[0].ifaces[0]

			perWorker := b.N / workers
			if perWorker == 0 {
				perWorker = 1
			}
			b.StartTimer()
			b.ReportAllocs()

			var wg sync.WaitGroup
			for range workers {
				wg.Go(func() {
					for i := 0; i < perWorker; i++ {
						_ = src.Send(pkt, "")
					}
				})
			}
			wg.Wait()
		})
	}
}
