// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"sync"
	"time"
)

const defaultQueueCapacity = 256

type eventQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	events []Event
	cap    int
	closed bool
}

func newEventQueue(capacity int) *eventQueue {
	if capacity <= 0 {
		capacity = defaultQueueCapacity
	}
	q := &eventQueue{
		events: make([]Event, 0, capacity),
		cap:    capacity,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *eventQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *eventQueue) push(ev Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	if len(q.events) >= q.cap {
		copy(q.events, q.events[1:])
		q.events = q.events[:len(q.events)-1]
	}
	q.events = append(q.events, cloneEvent(ev))
	q.cond.Signal()
}

func (q *eventQueue) poll(timeout time.Duration) (Event, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	tryDequeue := func() (Event, bool) {
		if len(q.events) == 0 {
			return Event{}, false
		}
		ev := q.events[0]
		q.events = q.events[1:]
		return ev, true
	}

	if ev, ok := tryDequeue(); ok {
		return ev, nil
	}
	if q.closed {
		return Event{}, errState
	}
	if timeout == 0 {
		return Event{}, errTimeout
	}

	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Event{}, errTimeout
		}
		timedOut := make(chan struct{})
		timer := time.AfterFunc(remaining, func() {
			q.cond.Broadcast()
			close(timedOut)
		})
		q.cond.Wait()
		timer.Stop()

		if ev, ok := tryDequeue(); ok {
			return ev, nil
		}
		if q.closed {
			return Event{}, errState
		}
		select {
		case <-timedOut:
			return Event{}, errTimeout
		default:
		}
	}
}

func cloneEvent(ev Event) Event {
	out := Event{
		Kind:         ev.Kind,
		Hops:         ev.Hops,
		Path:         ev.Path,
		ErrorMessage: ev.ErrorMessage,
	}
	if ev.LinkID != nil {
		out.LinkID = append([]byte(nil), ev.LinkID...)
	}
	if ev.DestinationHash != nil {
		out.DestinationHash = append([]byte(nil), ev.DestinationHash...)
	}
	if ev.IdentityHash != nil {
		out.IdentityHash = append([]byte(nil), ev.IdentityHash...)
	}
	if ev.RequestID != nil {
		out.RequestID = append([]byte(nil), ev.RequestID...)
	}
	if ev.AppData != nil {
		out.AppData = append([]byte(nil), ev.AppData...)
	}
	return out
}
