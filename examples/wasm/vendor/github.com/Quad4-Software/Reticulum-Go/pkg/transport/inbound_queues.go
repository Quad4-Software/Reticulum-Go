// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"sync"
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

// inboundQueues is a priority queue backend for inbound transport packets.
type inboundQueues struct {
	mu      sync.Mutex
	cond    sync.Cond
	queues  [inboundQueueCount][]packetJob
	sizes   [inboundQueueCount]int
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
	q := &inboundQueues{sizes: sizes}
	q.cond.L = &q.mu
	return q
}

func (q *inboundQueues) put(tc int, job packetJob) bool {
	if q == nil || tc < 0 || tc >= inboundQueueCount {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queues[tc]) >= q.sizes[tc] {
		q.dropped[tc]++
		putPacketCopy(job.pc)
		return false
	}
	q.queues[tc] = append(q.queues[tc], job)
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
			if len(q.queues[tc]) > 0 {
				job := q.queues[tc][0]
				q.queues[tc] = q.queues[tc][1:]
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
		if len(q.queues[tc]) < q.sizes[tc] {
			q.queues[tc] = append(q.queues[tc], job)
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
		heights[i] = len(q.queues[i])
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
}

func (t *Transport) inboundQueueDrainer() {
	defer t.inboundWG.Done()
	for {
		job, ok := t.inboundQueues.get(t.done)
		if !ok {
			return
		}
		if !t.enqueuePacket(job, false) {
			putPacketCopy(job.pc)
			t.shedHandlerOverflow(job.iface)
		}
	}
}
