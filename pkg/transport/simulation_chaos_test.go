// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"fmt"
	"math/rand/v2"
	"runtime"
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

// corruptMode selects how corruptSimIface mutates outbound frames.
type corruptMode int

const (
	corruptBitFlip corruptMode = iota
	corruptTruncate
	corruptDuplicate
	corruptReorder
)

// corruptSimIface mutates, duplicates, or reorders outbound frames with a
// seeded RNG so chaos tests hit parser and path-table surprises.
type corruptSimIface struct {
	*simIface
	mode     corruptMode
	prob     float64
	rng      *rand.Rand
	rngMu    sync.Mutex
	queue    [][]byte
	queueCap int
	mutated  uint64
}

func linkCorrupt(t testing.TB, s *simNetwork, a, b int, mode corruptMode, prob float64, seed uint64) (*corruptSimIface, *corruptSimIface) {
	t.Helper()
	if a == b {
		t.Fatalf("cannot link node %d to itself", a)
	}
	na, nb := s.nodes[a], s.nodes[b]
	leftBase := newSimIface(fmt.Sprintf("%s_corrupt_%s", na.name, nb.name))
	rightBase := newSimIface(fmt.Sprintf("%s_corrupt_%s", nb.name, na.name))
	leftBase.peer = rightBase
	rightBase.peer = leftBase
	left := &corruptSimIface{
		simIface: leftBase,
		mode:     mode,
		prob:     prob,
		rng:      rand.New(rand.NewPCG(seed, seed^0xc0ffee)),
		queueCap: 8,
	}
	right := &corruptSimIface{
		simIface: rightBase,
		mode:     mode,
		prob:     prob,
		rng:      rand.New(rand.NewPCG(seed^0xbadc0de, seed^0xface)),
		queueCap: 8,
	}
	if err := na.tr.RegisterInterface(leftBase.GetName(), left); err != nil {
		t.Fatalf("register corrupt left: %v", err)
	}
	if err := nb.tr.RegisterInterface(rightBase.GetName(), right); err != nil {
		t.Fatalf("register corrupt right: %v", err)
	}
	na.tr.ifaceStates.put(leftBase.GetName(), &ifaceState{})
	nb.tr.ifaceStates.put(rightBase.GetName(), &ifaceState{})
	na.ifaces = append(na.ifaces, leftBase)
	nb.ifaces = append(nb.ifaces, rightBase)
	s.addEdge(a, b)
	return left, right
}

func (c *corruptSimIface) shouldMutate() bool {
	c.rngMu.Lock()
	defer c.rngMu.Unlock()
	return c.rng.Float64() < c.prob
}

func (c *corruptSimIface) deliver(data []byte) {
	if c.peer == nil {
		return
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case c.peer.inbox <- cp:
	case <-c.peer.done:
	case <-c.done:
	}
}

func (c *corruptSimIface) Send(data []byte, addr string) error {
	if id := c.GetIFAC(); id != nil {
		masked, err := id.Mask(data)
		if err != nil {
			return err
		}
		data = masked
	}
	c.Mutex.Lock()
	c.TxBytes += uint64(len(data))
	c.TxPackets++
	c.Mutex.Unlock()
	if c.peer == nil {
		return nil
	}

	out := data
	switch c.mode {
	case corruptBitFlip:
		if len(out) > 0 && c.shouldMutate() {
			cp := append([]byte(nil), out...)
			c.rngMu.Lock()
			i := c.rng.IntN(len(cp))
			cp[i] ^= byte(1 + c.rng.IntN(255))
			c.rngMu.Unlock()
			out = cp
			atomic.AddUint64(&c.mutated, 1)
		}
		c.deliver(out)
	case corruptTruncate:
		if len(out) > 2 && c.shouldMutate() {
			c.rngMu.Lock()
			n := 1 + c.rng.IntN(len(out)-1)
			c.rngMu.Unlock()
			out = out[:n]
			atomic.AddUint64(&c.mutated, 1)
		}
		c.deliver(out)
	case corruptDuplicate:
		c.deliver(out)
		if c.shouldMutate() {
			c.deliver(out)
			atomic.AddUint64(&c.mutated, 1)
		}
	case corruptReorder:
		cp := append([]byte(nil), out...)
		c.rngMu.Lock()
		c.queue = append(c.queue, cp)
		flush := len(c.queue) >= c.queueCap || c.rng.Float64() < 0.35
		var batch [][]byte
		if flush {
			batch = c.queue
			c.queue = nil
			c.rng.Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })
			atomic.AddUint64(&c.mutated, 1)
		}
		c.rngMu.Unlock()
		for _, frame := range batch {
			c.deliver(frame)
		}
	default:
		c.deliver(out)
	}
	return nil
}

func (c *corruptSimIface) flushReorderLocked() {
	if len(c.queue) == 0 {
		return
	}
	batch := c.queue
	c.queue = nil
	c.rng.Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })
	for _, frame := range batch {
		c.deliver(frame)
	}
}

func (c *corruptSimIface) stop() {
	c.rngMu.Lock()
	c.flushReorderLocked()
	c.rngMu.Unlock()
	c.simIface.stop()
}

// TestSimChaosCorruptFrames bit-flips and truncates frames on a line and
// asserts announce flood does not deadlock or leak goroutines.
func TestSimChaosCorruptFrames(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseG := runtime.NumGoroutine()

	const n = 5
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)
	net.link(t, 0, 1)
	leftFlip, _ := linkCorrupt(t, net, 1, 2, corruptBitFlip, 0.35, 0xc0ffee01)
	leftTrunc, _ := linkCorrupt(t, net, 2, 3, corruptTruncate, 0.25, 0xc0ffee02)
	net.link(t, 3, 4)

	src := net.nodes[0]
	src.originateAnnounce(t)
	took, ok := waitForPaths(net.nodes[1:], src.destHash, 12*time.Second)
	t.Logf("corrupt frames: %d/%d paths after %v flips=%d truncs=%d",
		ok, n-1, took, atomic.LoadUint64(&leftFlip.mutated), atomic.LoadUint64(&leftTrunc.mutated))
	if ok == 0 {
		t.Fatal("corrupt frames: zero paths learned (possible deadlock or total drop)")
	}

	net.close()
	runtime.GC()
	time.Sleep(400 * time.Millisecond)
	finalG := runtime.NumGoroutine()
	if finalG > baseG+8 {
		t.Fatalf("goroutine leak after corrupt chaos: baseline=%d final=%d", baseG, finalG)
	}
}

// TestSimChaosDuplicateReorder duplicates and reorders frames. Delivery may
// still converge. The oracle is no deadlock and some path learning.
func TestSimChaosDuplicateReorder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chaos simulation in -short mode")
	}
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseG := runtime.NumGoroutine()

	const n = 4
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)
	leftDup, _ := linkCorrupt(t, net, 0, 1, corruptDuplicate, 0.4, 0xd015e01)
	leftRe, _ := linkCorrupt(t, net, 1, 2, corruptReorder, 1.0, 0xd015e02)
	net.link(t, 2, 3)

	src := net.nodes[0]
	src.originateAnnounce(t)
	took, ok := waitForPaths(net.nodes[1:], src.destHash, 12*time.Second)
	t.Logf("duplicate/reorder: %d/%d paths after %v dups=%d reorders=%d",
		ok, n-1, took, atomic.LoadUint64(&leftDup.mutated), atomic.LoadUint64(&leftRe.mutated))
	if ok < 1 {
		t.Fatal("duplicate/reorder: no path learning (possible deadlock)")
	}

	net.close()
	runtime.GC()
	time.Sleep(400 * time.Millisecond)
	finalG := runtime.NumGoroutine()
	if finalG > baseG+8 {
		t.Fatalf("goroutine leak after duplicate/reorder: baseline=%d final=%d", baseG, finalG)
	}
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
