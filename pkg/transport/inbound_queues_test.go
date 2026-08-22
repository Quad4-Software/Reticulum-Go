// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"sync"
	"testing"
	"time"
)

func TestInboundQueuesPriorityOrder(t *testing.T) {
	q := newInboundQueues([inboundQueueCount]int{4, 4, 4, 4})
	pcAnn := getPacketCopy(2)
	pcAnn.buf[0] = PacketTypeAnnounce
	if !q.put(TCAnnounce, packetJob{pc: pcAnn, packetType: PacketTypeAnnounce}) {
		t.Fatal("put announce")
	}
	pcData := getPacketCopy(2)
	pcData.buf[0] = 0x00
	if !q.put(TCData, packetJob{pc: pcData, packetType: 0}) {
		t.Fatal("put data")
	}
	done := make(chan struct{})
	job, ok := q.get(done)
	if !ok {
		t.Fatal("no job")
	}
	if job.packetType != 0 {
		t.Fatalf("expected data first (Python queue scan order), got type 0x%02x", job.packetType)
	}
	putPacketCopy(job.pc)
	job2, ok := q.get(done)
	if !ok || job2.packetType != PacketTypeAnnounce {
		t.Fatalf("second job=%v ok=%v", job2.packetType, ok)
	}
	putPacketCopy(job2.pc)
}

func TestInboundQueuesDropWhenFull(t *testing.T) {
	q := newInboundQueues([inboundQueueCount]int{1, 1, 1, 1})
	pc := getPacketCopy(2)
	if !q.put(TCData, packetJob{pc: pc, packetType: 0}) {
		t.Fatal("first put")
	}
	pc2 := getPacketCopy(2)
	if q.put(TCData, packetJob{pc: pc2, packetType: 0}) {
		t.Fatal("expected drop")
	}
	_, heights, dropped := q.snapshot()
	if heights[TCData] != 1 {
		t.Fatalf("height=%d", heights[TCData])
	}
	if dropped[TCData] != 1 {
		t.Fatalf("dropped=%d", dropped[TCData])
	}
}

func TestInboundQueuesSnapshotTotals(t *testing.T) {
	q := newInboundQueues([inboundQueueCount]int{8, 8, 8, 8})
	for range 3 {
		pc := getPacketCopy(2)
		_ = q.put(TCData, packetJob{pc: pc, packetType: 0})
	}
	total, heights, _ := q.snapshot()
	if total != 3 || heights[TCData] != 3 {
		t.Fatalf("total=%d heights=%v", total, heights)
	}
}

func TestInboundQueuesWakeOnClose(t *testing.T) {
	q := newInboundQueues([inboundQueueCount]int{4, 4, 4, 4})
	done := make(chan struct{})
	close(done)
	_, ok := q.get(done)
	if ok {
		t.Fatal("expected false after close")
	}
}

func TestPathRequestDestinationHashStable(t *testing.T) {
	h1 := pathRequestDestinationHash()
	h2 := pathRequestDestinationHash()
	if len(h1) != 16 {
		t.Fatalf("len=%d", len(h1))
	}
	if string(h1) != string(h2) {
		t.Fatal("hash not stable")
	}
}

func TestTransportInboundQueueStatsRPC(t *testing.T) {
	tr := NewTransport(nil)
	defer tr.Close()
	pc := getPacketCopy(4)
	pc.buf[0] = 0x00
	pc.buf[1] = 0x00
	if !tr.inboundQueues.put(TCData, packetJob{pc: pc, packetType: 0}) {
		t.Fatal("put")
	}
	total, _, _ := tr.inboundQueueSnapshot()
	if total < 1 {
		t.Fatalf("queue height=%d want >=1", total)
	}
	stats := tr.GetInterfaceStatsRPC()
	if stats.RXQT < 1 {
		t.Fatalf("rxqt=%d want >=1", stats.RXQT)
	}
}

func TestInboundQueuesRace(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	q := newInboundQueues([inboundQueueCount]int{256, 64, 64, 32})
	done := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 500 {
				job, ok := q.get(done)
				if !ok {
					return
				}
				putPacketCopy(job.pc)
			}
		})
	}
	for i := range 2000 {
		pc := getPacketCopy(2)
		pc.buf[0] = byte(i % 4)
		tc := i % inboundQueueCount
		_ = q.put(tc, packetJob{pc: pc, packetType: pc.buf[0]})
	}
	close(done)
	q.wakeAll()
	wg.Wait()
}

func TestInboundQueueDrainerShutdown(t *testing.T) {
	tr := NewTransport(nil)
	time.Sleep(20 * time.Millisecond)
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
}
