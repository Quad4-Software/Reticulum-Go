// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"testing"
	"testing/quick"
)

func TestEventQueuePropertyDropOldest(t *testing.T) {
	fn := func(capRaw uint8, pushes uint16) bool {
		queueCap := int(capRaw%32) + 1
		n := int(pushes%200) + 1
		q := newEventQueue(queueCap)
		for i := 0; i < n; i++ {
			q.push(Event{Kind: EventAnnounce, Hops: uint8(i % 256)})
		}
		expected := n
		if expected > queueCap {
			expected = queueCap
		}
		got := 0
		firstHops := -1
		for {
			ev, err := q.poll(0)
			if err != nil {
				break
			}
			if firstHops < 0 {
				firstHops = int(ev.Hops)
			}
			got++
		}
		if got != expected {
			return false
		}
		if n > queueCap {
			wantFirst := (n - queueCap) % 256
			if firstHops != wantFirst {
				return false
			}
		}
		return true
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTableProperty(t *testing.T) {
	fn := func(ops []byte) bool {
		tbl := newHandleTable()
		type entry struct {
			id   uint64
			kind handleKind
		}
		var live []entry
		kinds := []handleKind{kindNode, kindIdentity, kindDestination, kindLink}
		for _, op := range ops {
			switch op % 4 {
			case 0:
				k := kinds[int(op)%len(kinds)]
				id := tbl.insert(k, struct{}{})
				live = append(live, entry{id: id, kind: k})
			case 1:
				if len(live) == 0 {
					continue
				}
				e := live[int(op)%len(live)]
				if _, err := tbl.get(e.id, e.kind); err != nil {
					return false
				}
				wrong := kinds[(int(e.kind))%len(kinds)]
				if wrong == e.kind {
					wrong = kinds[(int(e.kind)+1)%len(kinds)]
				}
				if _, err := tbl.get(e.id, wrong); err == nil {
					return false
				}
			case 2:
				if len(live) == 0 {
					continue
				}
				idx := int(op) % len(live)
				e := live[idx]
				if !tbl.delete(e.id) {
					return false
				}
				if tbl.delete(e.id) {
					return false
				}
				live = append(live[:idx], live[idx+1:]...)
			case 3:
				if _, err := tbl.get(uint64(op)+1, kindNode); err == nil {
					// may legitimately exist; only fail if not in live
					found := false
					for _, e := range live {
						if e.id == uint64(op)+1 {
							found = true
							break
						}
					}
					if !found {
						return false
					}
				}
			}
		}
		return true
	}
	if err := quick.Check(fn, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}
