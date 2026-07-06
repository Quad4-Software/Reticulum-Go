// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestSimRingConvergence(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 16
	net := buildRing(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	want := net.nodes[1:]
	timeout := simConvergenceTimeout(net.diameter())
	took := assertAllHavePath(t, want, src.destHash, timeout)
	t.Logf("ring(N=%d) converged in %v (%s)", n, took, formatSimTimeout(net))

	wantHops := make(map[int]uint8, n)
	for i := 1; i < n; i++ {
		if d, ok := net.shortestPath(i, 0); ok {
			wantHops[i] = uint8(d)
		}
	}
	assertHopCounts(t, net, 0, src.destHash, wantHops)
}

func TestSimRandomGraphConvergence(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 32
	net := buildRandom(t, n, 0.08, 0xc0ffee)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	timeout := simConvergenceTimeout(net.diameter())
	took := assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)
	t.Logf("random(N=%d) converged in %v (%s)", n, took, formatSimTimeout(net))
}

func TestSimMultiAnnouncerStar(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 20
	net := buildStar(t, n)
	t.Cleanup(net.close)

	for _, node := range net.nodes[1:] {
		node.originateAnnounce(t)
	}

	timeout := simConvergenceTimeout(2)
	for i := 1; i < n; i++ {
		took := assertAllHavePath(t, net.nodes[0:1], net.nodes[i].destHash, timeout)
		t.Logf("hub learned leaf %d in %v", i, took)
	}

	for i := 1; i < n; i++ {
		var others []*simNode
		for j := 1; j < n; j++ {
			if j != i {
				others = append(others, net.nodes[j])
			}
		}
		took := assertAllHavePath(t, others, net.nodes[i].destHash, timeout)
		t.Logf("leaf %d reached %d peers in %v", i, len(others), took)
	}
}

func TestSimLineHopIntegrity(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 32
	net := buildLine(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	timeout := simConvergenceTimeout(n - 1)
	took := assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)
	t.Logf("line(N=%d) converged in %v", n, took)

	assertHopCounts(t, net, 0, src.destHash, lineGraphHopMap(n, 0))
}

func TestSimNextHopNeighborLine(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 16
	net := buildLine(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	timeout := simConvergenceTimeout(n - 1)
	assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)

	for i := 1; i < n; i++ {
		assertNextHopTowardSourceOnLine(t, net, i, 0, src.destHash)
	}
}

func TestSimPathStaleAfterChurn(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 16
	net := buildLine(t, n)
	t.Cleanup(net.close)

	src := net.nodes[0]
	src.originateAnnounce(t)
	timeout := simConvergenceTimeout(n - 1)
	assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)

	victim := 8
	for _, ifc := range net.nodes[victim].ifaces {
		ifc.stop()
	}
	_ = net.nodes[victim].tr.Close()

	tail := net.nodes[n-1]
	tail.originateAnnounce(t)

	right := net.nodes[victim+1 : n-1]
	assertAllHavePath(t, right, tail.destHash, timeout)
	assertNoPath(t, net.nodes[:victim], tail.destHash)

	if src.tr.HasPath(tail.destHash) {
		t.Logf("head retained pre-churn path to tail (stale path may still be present)")
	}
}

func TestSimPartitionHeal(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	net := newSimNetwork(t, 6)
	t.Cleanup(net.close)
	for i := range 2 {
		net.link(t, i, i+1)
	}
	for i := 3; i < 5; i++ {
		net.link(t, i, i+1)
	}

	src := net.nodes[0]
	src.originateAnnounce(t)
	timeout := simConvergenceTimeout(2)
	assertAllHavePath(t, net.nodes[1:3], src.destHash, timeout)
	assertNoPath(t, net.nodes[3:], src.destHash)

	net.link(t, 2, 3)
	src.originateAnnounce(t)
	assertAllHavePath(t, net.nodes[3:], src.destHash, timeout)
}

func TestSimAsymmetricLossRandom(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 12
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)
	rng := rand.New(rand.NewPCG(0xbadcafe, 0xfeedface))
	net.link(t, 0, 1)
	linkLossy(t, net, 1, 2, 0.15, 0.05, 0x10cc)
	for i := 2; i < n-1; i++ {
		net.link(t, i, i+1)
	}
	for i := range n {
		for j := i + 2; j < n; j++ {
			if rng.Float64() < 0.15 {
				net.link(t, i, j)
			}
		}
	}

	src := net.nodes[0]
	src.originateAnnounce(t)
	timeout := simConvergenceTimeout(net.diameter()) + 10*time.Second
	took := assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)
	t.Logf("random+loss converged in %v", took)
}

func TestSimLineConvergenceRealTiming(t *testing.T) {
	skipSimIfShort(t)

	const n = 8
	net := buildLine(t, n)
	t.Cleanup(net.close)
	src := net.nodes[0]
	src.originateAnnounce(t)

	timeout := simConvergenceTimeout(n - 1)
	took := assertAllHavePath(t, net.nodes[1:], src.destHash, timeout)
	t.Logf("line(N=%d) real-timing converged in %v", n, took)
	assertHopCounts(t, net, 0, src.destHash, lineGraphHopMap(n, 0))
}
