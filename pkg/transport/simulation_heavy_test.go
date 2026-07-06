//go:build heavy

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"math/rand/v2"
	"testing"
	"time"
)

func simHeavyConvergenceTimeout(net *simNetwork) time.Duration {
	n := len(net.nodes)
	if simFastPathActive() {
		return time.Duration(n)*150*time.Millisecond + 60*time.Second
	}
	return simConvergenceTimeout(net.diameter()) + time.Duration(n)*time.Second
}

func TestSimHeavyRandomConvergence(t *testing.T) {
	enableSimFastPath(t)

	const n = 128
	net := buildRandom(t, n, 0.05, 0x1eaf001)
	t.Cleanup(net.close)

	for _, node := range net.nodes {
		node.originateAnnounce(t)
		time.Sleep(5 * time.Millisecond)
	}

	timeout := simHeavyConvergenceTimeout(net)
	assertAllHavePath(t, net.nodes[1:], net.nodes[0].destHash, timeout)
	assertAllHavePath(t, net.nodes[:n-1], net.nodes[n-1].destHash, timeout)

	rng := rand.New(rand.NewPCG(0x1eaf002, 0x1eaf003))
	for range 20 {
		src := rng.IntN(n)
		dst := rng.IntN(n)
		if src == dst {
			continue
		}
		if !net.nodes[dst].tr.HasPath(net.nodes[src].destHash) {
			t.Fatalf("node %d missing path to node %d after flood", dst, src)
		}
	}
	t.Logf("heavy random(N=%d) sample pairs ok (%s)", n, formatSimTimeout(net))
}

func TestSimHeavyLineDiameter(t *testing.T) {
	enableSimFastPath(t)

	const n = 128
	net := buildLine(t, n)
	t.Cleanup(net.close)

	src := net.nodes[0]
	tail := net.nodes[n-1]
	src.originateAnnounce(t)

	timeout := simHeavyConvergenceTimeout(net)
	took := assertAllHavePath(t, []*simNode{tail}, src.destHash, timeout)
	got := tail.tr.HopsTo(src.destHash)
	want := uint8(n - 1)
	if got != want {
		t.Fatalf("tail hopsTo(head) = %d, want %d", got, want)
	}
	t.Logf("heavy line(N=%d) tail converged in %v hops=%d", n, took, got)
}

func TestSimHeavyConcurrentAnnounces(t *testing.T) {
	enableSimFastPath(t)

	const n = 64
	const originators = 16
	net := buildRandom(t, n, 0.06, 0x1eaf004)
	t.Cleanup(net.close)

	for i := range originators {
		net.nodes[i].originateAnnounce(t)
	}

	timeout := simHeavyConvergenceTimeout(net)
	for i := range originators {
		dest := net.nodes[i].destHash
		peers := make([]*simNode, 0, n-1)
		for j, node := range net.nodes {
			if j != i {
				peers = append(peers, node)
			}
		}
		assertAllHavePath(t, peers, dest, timeout)
	}
	t.Logf("heavy concurrent announces: %d originators, all %d nodes converged", originators, n)
}
