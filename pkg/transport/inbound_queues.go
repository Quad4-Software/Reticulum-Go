// SPDX-License-Identifier: LicenseRef-Reticulum
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/Quad4-Software/Reticulum-Go/pkg/packet"
)

// Inbound traffic classes (RNS 1.5.0 Transport.TC_*).
const (
	TCData            = 0
	TCAnnounce        = 1
	TCPathRequest     = 2
	TCIngressLimited  = 3
	inboundQueueCount = 4
)

// Default inbound queue lengths match Python RNS 1.5.1+ Transport.INBOUND_*_QUEUE_LENGTH.
const (
	defaultInboundDataQueueLen     = 1024
	defaultInboundAnnounceQueueLen = 128
	defaultInboundPRQueueLen       = 128
	defaultInboundILQueueLen       = 8
)

// jobRing is a fixed-capacity FIFO for one traffic class. The backing array
// is allocated once at the configured queue length, so sustained traffic
// never reallocates or copies the way slice-queue pop-front reslicing did.
type jobRing struct {
	buf  []packetJob
	head int
	len  int
}

func (r *jobRing) full() bool { return r.len == len(r.buf) }

func (r *jobRing) push(job packetJob) {
	r.buf[(r.head+r.len)%len(r.buf)] = job
	r.len++
}

func (r *jobRing) pop() packetJob {
	job := r.buf[r.head]
	r.buf[r.head] = packetJob{}
	r.head = (r.head + 1) % len(r.buf)
	r.len--
	return job
}

// inboundQueues is a priority queue backend for inbound transport packets.
type inboundQueues struct {
	mu      sync.Mutex
	cond    sync.Cond
	queues  [inboundQueueCount]jobRing
	dropped [inboundQueueCount]uint64
}

func newInboundQueues(sizes [inboundQueueCount]int) *inboundQueues {
	for i := range sizes {
		if sizes[i] <= 0 {
			switch i {
			case TCData:
				sizes[i] = defaultInboundDataQueueLen
			case TCAnnounce:
				sizes[i] = defaultInboundAnnounceQueueLen
			case TCPathRequest:
				sizes[i] = defaultInboundPRQueueLen
			case TCIngressLimited:
				sizes[i] = defaultInboundILQueueLen
			}
		}
	}
	q := &inboundQueues{}
	for i, n := range sizes {
		q.queues[i].buf = make([]packetJob, n)
	}
	q.cond.L = &q.mu
	return q
}

func (q *inboundQueues) put(tc int, job packetJob) bool {
	if q == nil || tc < 0 || tc >= inboundQueueCount {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queues[tc].full() {
		q.dropped[tc]++
		return false
	}
	q.queues[tc].push(job)
	q.cond.Signal()
	return true
}

func (q *inboundQueues) get(done <-chan struct{}) (packetJob, bool) {
	if q == nil {
		return packetJob{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		for tc := range inboundQueueCount {
			if q.queues[tc].len > 0 {
				job := q.queues[tc].pop()
				// A slot just freed. Wake producers parked in putWait or
				// HandlePacketBlocking callers sleep forever once the queue
				// has space and no further puts arrive to signal them.
				q.cond.Broadcast()
				return job, true
			}
		}
		if done != nil {
			select {
			case <-done:
				return packetJob{}, false
			default:
			}
		}
		q.cond.Wait()
		if done != nil {
			select {
			case <-done:
				return packetJob{}, false
			default:
			}
		}
	}
}

func (q *inboundQueues) wakeAll() {
	if q == nil {
		return
	}
	q.mu.Lock()
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *inboundQueues) putWait(tc int, job packetJob, done <-chan struct{}) bool {
	if q == nil || tc < 0 || tc >= inboundQueueCount {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if !q.queues[tc].full() {
			q.queues[tc].push(job)
			q.cond.Signal()
			return true
		}
		if done != nil {
			select {
			case <-done:
				return false
			default:
			}
		}
		q.cond.Wait()
		if done != nil {
			select {
			case <-done:
				return false
			default:
			}
		}
	}
}

func (q *inboundQueues) snapshot() (total int, heights [inboundQueueCount]int, dropped [inboundQueueCount]uint64) {
	if q == nil {
		return 0, heights, dropped
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range inboundQueueCount {
		heights[i] = q.queues[i].len
		total += heights[i]
		dropped[i] = q.dropped[i]
	}
	return total, heights, dropped
}

func inboundQueueSizesFromConfig(cfg inboundQueueConfig) [inboundQueueCount]int {
	var sizes [inboundQueueCount]int
	if cfg != nil {
		if v := cfg.InboundDataQueueLength(); v > 0 {
			sizes[TCData] = v
		}
		if v := cfg.InboundAnnounceQueueLength(); v > 0 {
			sizes[TCAnnounce] = v
		}
		if v := cfg.InboundPRQueueLength(); v > 0 {
			sizes[TCPathRequest] = v
		}
		if v := cfg.InboundILQueueLength(); v > 0 {
			sizes[TCIngressLimited] = v
		}
	}
	return sizes
}

type inboundQueueConfig interface {
	InboundDataQueueLength() int
	InboundAnnounceQueueLength() int
	InboundPRQueueLength() int
	InboundILQueueLength() int
}

func (t *Transport) inboundQueueSnapshot() (total int, heights [inboundQueueCount]int, dropped [inboundQueueCount]uint64) {
	if t == nil || t.inboundQueues == nil {
		return 0, heights, dropped
	}
	return t.inboundQueues.snapshot()
}

func (t *Transport) startInboundDrainer() {
	if t == nil || t.inboundQueues == nil {
		return
	}
	t.inboundWG.Add(1)
	go t.inboundQueueDrainer()
	for _, lane := range t.linkLanes {
		t.inboundWG.Add(1)
		go t.linkDataLaneWorker(lane)
	}
	for _, lane := range t.announceLanes {
		t.inboundWG.Add(1)
		go t.linkDataLaneWorker(lane)
	}
}

// linkDataLaneCount sizes the per-link data lanes. Link crypto is the
// CPU-bound part of the steady-state path, so lanes track available
// parallelism but stay bounded to keep goroutine count sane on big hosts.
func linkDataLaneCount() int {
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		n = 2
	}
	if n > 8 {
		n = 8
	}
	return n
}

// linkDataLaneWorker drains one link-data lane. Jobs for the same link id
// always land on the same lane, so per-link wire order is preserved while
// different links process in parallel.
func (t *Transport) linkDataLaneWorker(lane <-chan packetJob) {
	defer t.inboundWG.Done()
	for {
		select {
		case <-t.done:
			return
		case job, ok := <-lane:
			if !ok {
				return
			}
			t.runPacketJob(job)
		}
	}
}

// announceLaneCount sizes the announce lanes. Announce handling ends in the
// process-global known-destinations lock, so lanes stay narrower than the
// link-data lanes: enough workers to keep signature verification parallel
// without parking dozens of goroutines on one mutex during an announce
// flood. Announce jobs that overflow shed like any other handler overflow.
func announceLaneCount() int {
	n := runtime.GOMAXPROCS(0)
	if n < 2 {
		n = 2
	}
	if n > 4 {
		n = 4
	}
	return n
}

// linkDataLaneFor maps a job to its lane by link id (the packet's
// destination hash, truncated the same way handleLinkPacket keys t.links).
func (t *Transport) linkDataLaneFor(job packetJob) chan packetJob {
	return laneForHash(t.linkLanes, job)
}

// announceLaneFor maps an announce job to its lane by announced destination
// hash, so announces for one destination stay ordered while different
// destinations verify in parallel.
func (t *Transport) announceLaneFor(job packetJob) chan packetJob {
	return laneForHash(t.announceLanes, job)
}

func laneForHash(lanes []chan packetJob, job packetJob) chan packetJob {
	if len(lanes) == 0 {
		return nil
	}
	if job.pkt == nil {
		return lanes[0]
	}
	dh := job.pkt.DestinationHash
	if len(dh) > 16 {
		dh = dh[:16]
	}
	k := hash16FromSlice(dh)
	// uint16 -> int cannot overflow on any supported platform, and two
	// hash bytes give plenty of spread across the small lane counts.
	idx := int(binary.LittleEndian.Uint16(k.bytes[:2])) % len(lanes)
	return lanes[idx]
}

func (t *Transport) inboundQueueDrainer() {
	defer t.inboundWG.Done()
	for {
		job, ok := t.inboundQueues.get(t.done)
		if !ok {
			return
		}
		// Data packets for established links are sharded across per-link-id
		// lanes so wire order survives to Link.HandleInbound while different
		// links decrypt in parallel. Routing them through the shared worker
		// channel let consecutive packets for one link be dequeued in order
		// but processed out of order, which broke identify-before-request
		// sequencing; a single serial lane capped multi-link throughput.
		if job.destType == DestTypeLink && job.packetType == packet.PacketTypeData {
			lane := t.linkDataLaneFor(job)
			if lane == nil {
				t.runPacketJob(job)
				continue
			}
			select {
			case <-t.done:
				putPacketCopy(job.pc)
				putInboundPacket(job.pkt)
				return
			case lane <- job:
			default:
				putPacketCopy(job.pc)
				putInboundPacket(job.pkt)
				t.shedHandlerOverflow(job.iface)
			}
			continue
		}
		// Announces get dedicated lanes for the same reason: signature
		// verification is CPU-bound and the remember path ends on a
		// process-global lock, so letting announce jobs occupy the shared
		// worker pool lets an announce flood starve every other packet type.
		if job.packetType == PacketTypeAnnounce {
			lane := t.announceLaneFor(job)
			if lane == nil {
				t.runPacketJob(job)
				continue
			}
			select {
			case <-t.done:
				putPacketCopy(job.pc)
				putInboundPacket(job.pkt)
				return
			case lane <- job:
			default:
				putPacketCopy(job.pc)
				putInboundPacket(job.pkt)
				t.shedHandlerOverflow(job.iface)
			}
			continue
		}
		if !t.enqueuePacket(job, false) {
			putPacketCopy(job.pc)
			putInboundPacket(job.pkt)
			t.shedHandlerOverflow(job.iface)
		}
	}
}
