// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestPathTableEmpty(t *testing.T) {
	node := mustCreateNode(t)
	rows, code := PathTable(node, -1)
	if code != OK {
		t.Fatal(code, LastError())
	}
	if rows == nil {
		t.Fatal("nil rows")
	}
}

func TestLifecyclePauseResumeRefresh(t *testing.T) {
	node := mustCreateNode(t)
	id := mustIdentity(t)
	if code := NodeSetIdentity(node, id); code != OK {
		t.Fatal(code)
	}
	if code := NodeStart(node); code != OK {
		t.Fatal(code)
	}
	t.Cleanup(func() { _ = NodeStop(node) })

	if code := NodePause(node); code != OK {
		t.Fatal(code, LastError())
	}
	if code := NodeResume(node); code != OK {
		t.Fatal(code, LastError())
	}
	if code := NodeRefreshPaths(node); code != OK {
		t.Fatal(code, LastError())
	}
}

func TestEventCallback(t *testing.T) {
	node := mustCreateNode(t)
	var mu sync.Mutex
	var got []Event
	if code := SetEventCallback(node, func(ev Event) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}); code != OK {
		t.Fatal(code)
	}
	t.Cleanup(func() { _ = SetEventCallback(node, nil) })

	rec, err := nodeByHandle(node)
	if err != nil {
		t.Fatal(err)
	}
	rec.enqueue(Event{Kind: EventAnnounce, Hops: 3, AppData: []byte("cb")})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 || got[0].Kind != EventAnnounce || !bytes.Equal(got[0].AppData, []byte("cb")) {
		t.Fatalf("callback events: %+v", got)
	}
}

func TestRequestRespondUnknown(t *testing.T) {
	node := mustCreateNode(t)
	if code := RequestRespond(node, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, nil); code != ErrNotFound {
		t.Fatalf("got %d want not found", code)
	}
}

func TestDestinationRegisterRequestHandler(t *testing.T) {
	node := mustCreateNode(t)
	id := mustIdentity(t)
	if code := NodeSetIdentity(node, id); code != OK {
		t.Fatal(code)
	}
	dest, code := DestinationCreate(node, 0, "reqapp", []string{"svc"}, false)
	if code != OK {
		t.Fatal(code, LastError())
	}
	t.Cleanup(func() { _ = DestinationDestroy(dest) })
	if code := DestinationRegisterRequestHandler(dest, "/ping"); code != OK {
		t.Fatal(code, LastError())
	}
	if code := DestinationRegisterRequestHandler(dest, ""); code != ErrInvalidArg {
		t.Fatalf("empty path: %d", code)
	}
}
