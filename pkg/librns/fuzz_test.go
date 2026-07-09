// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"sync"
	"testing"
	"time"
)

func FuzzHandleTable(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Add([]byte{1, 1, 1, 1, 0, 2, 3})
	f.Fuzz(func(t *testing.T, ops []byte) {
		tbl := newHandleTable()
		var live []uint64
		kinds := []handleKind{kindNode, kindIdentity, kindDestination, kindLink}
		for _, op := range ops {
			switch op % 5 {
			case 0:
				k := kinds[int(op)%len(kinds)]
				id := tbl.insert(k, struct{}{})
				live = append(live, id)
			case 1:
				if len(live) == 0 {
					continue
				}
				id := live[int(op)%len(live)]
				_, _ = tbl.get(id, kinds[int(op)%len(kinds)])
			case 2:
				if len(live) == 0 {
					continue
				}
				idx := int(op) % len(live)
				id := live[idx]
				_ = tbl.delete(id)
				live = append(live[:idx], live[idx+1:]...)
			case 3:
				_ = tbl.delete(uint64(op) + 1)
			case 4:
				if len(live) == 0 {
					continue
				}
				id := live[int(op)%len(live)]
				_ = tbl.delete(id)
				_ = tbl.delete(id)
			}
		}
	})
}

func FuzzEventQueue(f *testing.F) {
	f.Add(uint8(0), int32(0), []byte{1, 2, 3})
	f.Add(uint8(7), int32(maxFuzzWaitMs), []byte{0, 1, 0, 1, 0})
	f.Fuzz(func(t *testing.T, seed uint8, waitMs int32, ops []byte) {
		queueCap := int(seed%16) + 1
		q := newEventQueue(queueCap)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i, b := range ops {
				if b%3 == 0 {
					q.close()
					return
				}
				q.push(Event{
					Kind:    EventAnnounce,
					Hops:    uint8(i),
					AppData: []byte{b},
				})
			}
		}()
		go func() {
			defer wg.Done()
			timeout := time.Duration(waitMs%maxFuzzWaitMs) * time.Millisecond
			for range ops {
				_, _ = q.poll(timeout)
			}
		}()
		wg.Wait()
		q.close()
		_, _ = q.poll(0)
	})
}

const maxFuzzWaitMs = 80

func FuzzConfigPathCreate(f *testing.F) {
	f.Add("")
	f.Add("config")
	f.Add("/no/such/path")
	f.Add("bad\x00path")
	f.Fuzz(func(t *testing.T, path string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on path %q: %v", path, r)
			}
		}()
		id, code := NodeCreate(path)
		if code == OK && id != 0 {
			_ = NodeDestroy(id)
		}
	})
}

func FuzzValidatePath(f *testing.F) {
	f.Add("")
	f.Add("ok")
	f.Add("a\x00b")
	f.Fuzz(func(t *testing.T, path string) {
		err := validatePath(path)
		if path == "" || containsNUL(path) {
			if err == nil {
				t.Fatalf("expected error for %q", path)
			}
			return
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", path, err)
		}
	})
}

func containsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}
