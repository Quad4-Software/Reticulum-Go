// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
package transport

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var simFastPathfinderRW = 0.0
var simFastAnnounceBypass = 0.0

func enableSimFastPath(t testing.TB) {
	t.Helper()
	simHooksMu.Lock()
	simPathfinderRW = &simFastPathfinderRW
	simAnnounceRateKbps = &simFastAnnounceBypass
	simHooksMu.Unlock()
	t.Cleanup(disableSimFastPath)
}

func disableSimFastPath() {
	simHooksMu.Lock()
	simPathfinderRW = nil
	simAnnounceRateKbps = nil
	simHooksMu.Unlock()
}

func simConvergenceTimeout(diameter int) time.Duration {
	if diameter < 1 {
		diameter = 1
	}
	if simFastPathActive() {
		return time.Duration(diameter)*50*time.Millisecond + 2*time.Second
	}
	return time.Duration(float64(diameter)*PathfinderRW*3)*time.Second + 5*time.Second
}

func skipSimIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping multi-node simulation in -short mode")
	}
}

func assertAllHavePath(t testing.TB, nodes []*simNode, dest []byte, timeout time.Duration) time.Duration {
	t.Helper()
	took, ok := waitForPaths(nodes, dest, timeout)
	if ok != len(nodes) {
		missing := make([]int, 0)
		for i, n := range nodes {
			if !n.tr.HasPath(dest) {
				missing = append(missing, i)
			}
		}
		t.Fatalf("convergence failed: %d/%d nodes after %v (missing indices %v)",
			ok, len(nodes), took, missing)
	}
	return took
}

func assertNoPath(t testing.TB, nodes []*simNode, dest []byte) {
	t.Helper()
	for i, n := range nodes {
		if n.tr.HasPath(dest) {
			t.Fatalf("node %d unexpectedly has path to %x", i, dest[:8])
		}
	}
}

func assertHopCounts(t testing.TB, net *simNetwork, srcIdx int, dest []byte, want map[int]uint8) {
	t.Helper()
	src := net.nodes[srcIdx]
	for idx, wantHops := range want {
		got := net.nodes[idx].tr.HopsTo(dest)
		if got != wantHops {
			t.Errorf("node%d hopsTo(%s) = %d, want %d", idx, src.name, got, wantHops)
		}
	}
}

func graphDistanceLine(n, srcIdx, dstIdx int) int {
	if srcIdx == dstIdx {
		return 0
	}
	d := srcIdx - dstIdx
	if d < 0 {
		d = -d
	}
	return d
}

func graphDistanceStar(n, srcIdx, dstIdx int) int {
	if srcIdx == dstIdx {
		return 0
	}
	if srcIdx == 0 || dstIdx == 0 {
		return 1
	}
	return 2
}

func (s *simNetwork) shortestPath(srcIdx, dstIdx int) (int, bool) {
	path, ok := s.pathNodes(srcIdx, dstIdx)
	if !ok {
		return 0, false
	}
	return len(path) - 1, true
}

func (s *simNetwork) pathNodes(srcIdx, dstIdx int) ([]int, bool) {
	if srcIdx < 0 || srcIdx >= len(s.nodes) || dstIdx < 0 || dstIdx >= len(s.nodes) {
		return nil, false
	}
	if srcIdx == dstIdx {
		return []int{srcIdx}, true
	}
	parent := make([]int, len(s.nodes))
	for i := range parent {
		parent[i] = -1
	}
	visited := make([]bool, len(s.nodes))
	queue := []int{srcIdx}
	visited[srcIdx] = true
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range s.adj[u] {
			if visited[v] {
				continue
			}
			visited[v] = true
			parent[v] = u
			if v == dstIdx {
				path := []int{dstIdx}
				for cur := dstIdx; parent[cur] >= 0; cur = parent[cur] {
					path = append([]int{parent[cur]}, path...)
				}
				return path, true
			}
			queue = append(queue, v)
		}
	}
	return nil, false
}

func preloadPathNodes(nodes []*simNode, path []int, target []byte) {
	for i := 0; i < len(path)-1; i++ {
		relay := nodes[path[i]]
		next := nodes[path[i+1]]
		if len(relay.ifaces) == 0 {
			continue
		}
		ifc := relay.ifaces[0]
		hops := uint8(len(path) - 1 - i)
		relay.tr.UpdatePath(target, next.id.Hash(), ifc.GetName(), hops)
	}
}

func (s *simNetwork) diameter() int {
	n := len(s.nodes)
	if n <= 1 {
		return 0
	}
	maxD := 0
	for i := range n {
		for j := i + 1; j < n; j++ {
			if d, ok := s.shortestPath(i, j); ok && d > maxD {
				maxD = d
			}
		}
	}
	return maxD
}

func assertNextHopTowardSourceOnLine(t testing.TB, net *simNetwork, nodeIdx, srcIdx int, dest []byte) {
	t.Helper()
	if nodeIdx <= srcIdx {
		return
	}
	ifName := net.nodes[nodeIdx].tr.NextHopInterface(dest)
	if ifName == "" {
		t.Fatalf("node%d has no next-hop interface toward dest", nodeIdx)
	}
	wantNeighbor := net.nodes[nodeIdx-1].name
	if !containsIfaceToward(net.nodes[nodeIdx], wantNeighbor) {
		t.Errorf("node%d nextHopInterface %q does not route toward %s", nodeIdx, ifName, wantNeighbor)
	}
}

func containsIfaceToward(node *simNode, peerName string) bool {
	want := fmt.Sprintf("_to_%s", peerName)
	for _, ifc := range node.ifaces {
		if strings.Contains(ifc.GetName(), want) {
			return true
		}
	}
	return false
}

func simPathTableLen(tr *Transport) int {
	tr.mutex.RLock()
	defer tr.mutex.RUnlock()
	return len(tr.paths)
}

func waitForRelayDelivery(t testing.TB, iface *simIface, startRx uint64, want uint64, timeout time.Duration) uint64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := iface.GetRxPackets()
		if got-startRx >= want {
			return got - startRx
		}
		time.Sleep(5 * time.Millisecond)
	}
	return iface.GetRxPackets() - startRx
}

func lineGraphHopMap(n, srcIdx int) map[int]uint8 {
	want := make(map[int]uint8, n)
	for i := range n {
		if i == srcIdx {
			continue
		}
		want[i] = uint8(graphDistanceLine(n, srcIdx, i))
	}
	return want
}

func formatSimTimeout(net *simNetwork) string {
	return fmt.Sprintf("diameter=%d timeout=%v fast=%v",
		net.diameter(), simConvergenceTimeout(net.diameter()), simFastPathActive())
}
