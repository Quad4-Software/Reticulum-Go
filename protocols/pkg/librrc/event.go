// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc

import (
	"sync"
	"time"
)

const defaultQueueCapacity = 256

const (
	EventWelcome = 1
	EventJoined  = 2
	EventParted  = 3
	EventMsg     = 4
	EventNotice  = 5
	EventAction  = 6
	EventError   = 7
	EventPong    = 8
	EventHello   = 9
	EventJoin    = 10
	EventPart    = 11
	EventClose   = 12
	EventTimeout = 13
)

type Event struct {
	Kind    int
	Sender  []byte
	Peer    []byte
	Room    string
	Nick    string
	Body    string
	MsgType uint64
}

type eventQueue struct {
	mu    sync.Mutex
	cond  *sync.Cond
	items []Event
	cap   int
}

func newEventQueue(capacity int) *eventQueue {
	if capacity <= 0 {
		capacity = defaultQueueCapacity
	}
	q := &eventQueue{cap: capacity}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *eventQueue) push(ev Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.cap {
		copy(q.items, q.items[1:])
		q.items = q.items[:len(q.items)-1]
	}
	q.items = append(q.items, ev)
	q.cond.Signal()
}

func (q *eventQueue) poll(timeout time.Duration) (Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	deadline := time.Now().Add(timeout)
	for len(q.items) == 0 {
		if timeout <= 0 {
			return Event{Kind: EventTimeout}, false
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Event{Kind: EventTimeout}, false
		}
		q.cond.Wait()
		if time.Now().After(deadline) {
			return Event{Kind: EventTimeout}, false
		}
	}
	ev := q.items[0]
	q.items = q.items[1:]
	return ev, true
}

func (q *eventQueue) clear() {
	q.mu.Lock()
	q.items = nil
	q.mu.Unlock()
}
