// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func uniqueRelayPayload(seq uint64) []byte {
	payload := make([]byte, 64)
	binary.BigEndian.PutUint64(payload, seq)
	return payload
}

func TestSimRelayDeliveryRatio(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 9
	const packets = 500
	const minDelivered = 495

	net := buildLine(t, n)
	t.Cleanup(net.close)
	target := net.nodes[n-1].id.Hash()
	preloadLinePaths(net.nodes, target)

	tail := net.nodes[n-1].ifaces[0]
	src := net.nodes[0].ifaces[0]
	second := net.nodes[1].id.Hash()

	startRx := tail.GetRxPackets()
	for i := range packets {
		pkt := buildHT2(second, target, 0, uniqueRelayPayload(uint64(i)))
		if err := src.Send(pkt, ""); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	got := waitForRelayDelivery(t, tail, startRx, packets, 10*time.Second)
	if got < minDelivered {
		t.Fatalf("delivery ratio too low: got %d want >= %d (%.1f%%)",
			got, minDelivered, float64(got)*100/float64(packets))
	}
	t.Logf("delivered %d/%d packets (%.1f%%)", got, packets, float64(got)*100/float64(packets))
}

func TestSimRelayDeliveryRatioConcurrent(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const n = 9
	const workers = 4
	const perWorker = 125
	const total = workers * perWorker
	const minDelivered = 396

	net := buildLine(t, n)
	t.Cleanup(net.close)
	target := net.nodes[n-1].id.Hash()
	preloadLinePaths(net.nodes, target)

	tail := net.nodes[n-1].ifaces[0]
	src := net.nodes[0].ifaces[0]
	second := net.nodes[1].id.Hash()
	startRx := tail.GetRxPackets()

	var seq atomic.Uint64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				pkt := buildHT2(second, target, 0, uniqueRelayPayload(seq.Add(1)))
				_ = src.Send(pkt, "")
			}
		})
	}
	wg.Wait()

	got := waitForRelayDelivery(t, tail, startRx, uint64(total), 15*time.Second)
	if got < minDelivered {
		t.Fatalf("concurrent delivery ratio too low: got %d want >= %d", got, minDelivered)
	}
	t.Logf("concurrent delivered %d/%d packets", got, total)
}

func TestSimRelayUnderLoss(t *testing.T) {
	skipSimIfShort(t)
	enableSimFastPath(t)

	const hops = 5
	const packets = 200
	const minDelivered = 160

	n := hops + 1
	net := newSimNetwork(t, n)
	t.Cleanup(net.close)
	net.link(t, 0, 1)
	linkLossy(t, net, 1, 2, 0.10, 0.0, 0xdead)
	for i := 2; i < n-1; i++ {
		net.link(t, i, i+1)
	}

	target := net.nodes[n-1].id.Hash()
	preloadLinePaths(net.nodes, target)

	tail := net.nodes[n-1].ifaces[0]
	src := net.nodes[0].ifaces[0]
	second := net.nodes[1].id.Hash()
	startRx := tail.GetRxPackets()

	for i := range packets {
		pkt := buildHT2(second, target, 0, uniqueRelayPayload(uint64(i)))
		_ = src.Send(pkt, "")
	}

	got := waitForRelayDelivery(t, tail, startRx, packets, 20*time.Second)
	if got < minDelivered {
		t.Fatalf("relay under loss: delivered %d/%d (want >= %d)", got, packets, minDelivered)
	}
	t.Logf("relay under 10%% loss: delivered %d/%d (%.1f%%)", got, packets, float64(got)*100/float64(packets))
}
