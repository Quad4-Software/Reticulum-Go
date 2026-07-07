// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// delaySimIface injects per-packet latency (and optional jitter) before
// delivery to the peer inbox.
type delaySimIface struct {
	*simIface
	baseDelay time.Duration
	jitter    time.Duration
	rng       *rand.Rand
	rngMu     sync.Mutex
	wg        sync.WaitGroup
}

func linkDelay(t testing.TB, s *simNetwork, a, b int, delayAtoB, delayBtoA time.Duration, jitter time.Duration, seed uint64) (*delaySimIface, *delaySimIface) {
	t.Helper()
	if a == b {
		t.Fatalf("cannot link node %d to itself", a)
	}
	na, nb := s.nodes[a], s.nodes[b]
	leftBase := newSimIface(fmt.Sprintf("%s_delay_%s", na.name, nb.name))
	rightBase := newSimIface(fmt.Sprintf("%s_delay_%s", nb.name, na.name))
	leftBase.peer = rightBase
	rightBase.peer = leftBase
	left := &delaySimIface{
		simIface:  leftBase,
		baseDelay: delayAtoB,
		jitter:    jitter,
		rng:       rand.New(rand.NewPCG(seed, seed^0xde1a)),
	}
	right := &delaySimIface{
		simIface:  rightBase,
		baseDelay: delayBtoA,
		jitter:    jitter,
		rng:       rand.New(rand.NewPCG(seed^0x1a2b, seed^0x3c4d)),
	}
	registerSimEdge(t, na, nb, leftBase, rightBase, left, right)
	s.addEdge(a, b)
	return left, right
}

func registerSimEdge(t testing.TB, na, nb *simNode, leftBase, rightBase *simIface, left, right *delaySimIface) {
	t.Helper()
	if err := na.tr.RegisterInterface(leftBase.GetName(), left); err != nil {
		t.Fatalf("register delay left: %v", err)
	}
	if err := nb.tr.RegisterInterface(rightBase.GetName(), right); err != nil {
		t.Fatalf("register delay right: %v", err)
	}
	na.tr.ifaceStates.put(leftBase.GetName(), &ifaceState{})
	nb.tr.ifaceStates.put(rightBase.GetName(), &ifaceState{})
	na.ifaces = append(na.ifaces, leftBase)
	nb.ifaces = append(nb.ifaces, rightBase)
}

func (d *delaySimIface) sampleDelay() time.Duration {
	delay := d.baseDelay
	if d.jitter > 0 {
		d.rngMu.Lock()
		j := time.Duration(d.rng.Int64N(int64(d.jitter)))
		d.rngMu.Unlock()
		delay += j
	}
	return delay
}

func (d *delaySimIface) Send(data []byte, addr string) error {
	if id := d.GetIFAC(); id != nil {
		masked, err := id.Mask(data)
		if err != nil {
			return err
		}
		data = masked
	}
	d.Mutex.Lock()
	d.TxBytes += uint64(len(data))
	d.TxPackets++
	d.Mutex.Unlock()
	if d.peer == nil {
		return nil
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	d.wg.Go(func() {
		timer := time.NewTimer(d.sampleDelay())
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-d.done:
			return
		case <-d.peer.done:
			return
		}
		select {
		case d.peer.inbox <- cp:
		case <-d.peer.done:
		case <-d.done:
		}
	})
	return nil
}

func (d *delaySimIface) stop() {
	d.simIface.stop()
	d.wg.Wait()
}

// flapSimIface drops all packets while the interface is marked down.
type flapSimIface struct {
	*simIface
	down atomic.Bool
}

func linkFlap(t testing.TB, s *simNetwork, a, b int) (*flapSimIface, *flapSimIface) {
	t.Helper()
	if a == b {
		t.Fatalf("cannot link node %d to itself", a)
	}
	na, nb := s.nodes[a], s.nodes[b]
	leftBase := newSimIface(fmt.Sprintf("%s_flap_%s", na.name, nb.name))
	rightBase := newSimIface(fmt.Sprintf("%s_flap_%s", nb.name, na.name))
	leftBase.peer = rightBase
	rightBase.peer = leftBase
	left := &flapSimIface{simIface: leftBase}
	right := &flapSimIface{simIface: rightBase}
	if err := na.tr.RegisterInterface(leftBase.GetName(), left); err != nil {
		t.Fatalf("register flap left: %v", err)
	}
	if err := nb.tr.RegisterInterface(rightBase.GetName(), right); err != nil {
		t.Fatalf("register flap right: %v", err)
	}
	na.tr.ifaceStates.put(leftBase.GetName(), &ifaceState{})
	nb.tr.ifaceStates.put(rightBase.GetName(), &ifaceState{})
	na.ifaces = append(na.ifaces, leftBase)
	nb.ifaces = append(nb.ifaces, rightBase)
	s.addEdge(a, b)
	return left, right
}

func (f *flapSimIface) Send(data []byte, addr string) error {
	if f.down.Load() {
		return nil
	}
	return f.simIface.Send(data, addr)
}

func (f *flapSimIface) setDown(down bool) {
	f.down.Store(down)
}

// TestSimChaosDelayAndLoss verifies announce convergence on a line with
// latency and asymmetric packet loss.
func TestSimChaosDelayAndLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	const n = 5
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)

	net.link(t, 0, 1)
	linkDelay(t, net, 1, 2, 8*time.Millisecond, 4*time.Millisecond, 4*time.Millisecond, 0xca05de1)
	left, _ := linkLossy(t, net, 2, 3, 0.25, 0.1, 0x1055)
	net.link(t, 3, 4)

	src := net.nodes[0]
	src.originateAnnounce(t)

	took, ok := waitForPaths(net.nodes[1:], src.destHash, 20*time.Second)
	if ok != n-1 {
		t.Fatalf("chaos delay+loss: %d/%d converged after %v (drops=%d)",
			ok, n-1, took, atomic.LoadUint64(&left.dropped))
	}
	t.Logf("chaos delay+loss converged in %v", took)
}

// TestSimChaosPartitionHeal partitions a line, heals it, and expects the
// tail to learn the originator path after healing.
func TestSimChaosPartitionHeal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	const n = 6
	net := newSimNetwork(t, n)
	var left, right *flapSimIface
	for i := range n - 1 {
		if i == 2 {
			left, right = linkFlap(t, net, 2, 3)
			continue
		}
		net.link(t, i, i+1)
	}
	t.Cleanup(net.close)
	src := net.nodes[0]
	tail := net.nodes[n-1]

	left.setDown(true)
	right.setDown(true)
	src.originateAnnounce(t)

	if _, ok := waitForPaths(net.nodes[1:3], src.destHash, 5*time.Second); ok != 2 {
		t.Fatalf("head segment did not converge before heal")
	}
	if tail.tr.HasPath(src.destHash) {
		t.Fatal("tail should not have path while partitioned")
	}

	left.setDown(false)
	right.setDown(false)
	src.originateAnnounce(t)

	took, ok := waitForPaths([]*simNode{tail}, src.destHash, 15*time.Second)
	if ok != 1 {
		t.Fatalf("tail did not converge after heal: %d/1 after %v", ok, took)
	}
	t.Logf("partition heal converged in %v", took)
}

// TestSimChaosInterfaceFlap flaps the middle link during announce flood.
func TestSimChaosInterfaceFlap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	const n = 8
	net := buildLine(t, n)
	t.Cleanup(net.close)
	left, right := linkFlap(t, net, 3, 4)

	src := net.nodes[0]
	go func() {
		for range 6 {
			left.setDown(true)
			right.setDown(true)
			time.Sleep(40 * time.Millisecond)
			left.setDown(false)
			right.setDown(false)
			time.Sleep(60 * time.Millisecond)
		}
	}()

	src.originateAnnounce(t)
	took, ok := waitForPaths(net.nodes[1:], src.destHash, 20*time.Second)
	if ok < n-2 {
		t.Fatalf("flap: only %d/%d converged after %v", ok, n-1, took)
	}
	t.Logf("interface flap: %d/%d converged in %v", ok, n-1, took)
}

// buildChaosMesh is a complete graph whose edges apply per-packet loss.
func buildChaosMesh(t testing.TB, n int, dropProb float64, seed uint64) *simNetwork {
	t.Helper()
	net := newSimNetwork(t, n)
	for i := range n {
		for j := i + 1; j < n; j++ {
			linkLossy(t, net, i, j, dropProb, dropProb, seed+uint64(i)<<16+uint64(j))
		}
	}
	return net
}

// TestSimChaosCombinedMesh exercises loss and mid-test interface churn on a
// small mesh.
func TestSimChaosCombinedMesh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	const n = 10
	net := buildChaosMesh(t, n, 0.08, 0xca05c0de)
	t.Cleanup(net.close)

	src := net.nodes[0]
	go func() {
		time.Sleep(150 * time.Millisecond)
		for _, idx := range []int{4, 7} {
			for _, ifc := range net.nodes[idx].ifaces {
				ifc.stop()
			}
		}
	}()

	src.originateAnnounce(t)
	took, ok := waitForPaths(net.nodes[1:], src.destHash, 25*time.Second)
	if ok < (n-1)*3/4 {
		t.Fatalf("combined chaos mesh: %d/%d converged after %v", ok, n-1, took)
	}
	t.Logf("combined chaos mesh: %d/%d converged in %v", ok, n-1, took)
}
