// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHandleLifecycle(t *testing.T) {
	node, code := NodeCreate("")
	if code != OK || node == 0 {
		t.Fatalf("NodeCreate: code=%d err=%q", code, LastError())
	}
	if code := NodeDestroy(node); code != OK {
		t.Fatalf("NodeDestroy: code=%d", code)
	}
	if code := NodeStart(node); code != ErrInvalidHandle {
		t.Fatalf("use after destroy: got %d want %d", code, ErrInvalidHandle)
	}
	if code := NodeDestroy(node); code != ErrInvalidHandle {
		t.Fatalf("double destroy: got %d want %d", code, ErrInvalidHandle)
	}
}

func TestNodeStartStopIdempotent(t *testing.T) {
	node := mustCreateNode(t)

	if code := NodeStart(node); code != OK {
		t.Fatal(code, LastError())
	}
	if code := NodeStart(node); code != OK {
		t.Fatalf("second start: %d", code)
	}
	if code := NodeStop(node); code != OK {
		t.Fatal(code)
	}
	if code := NodeStop(node); code != OK {
		t.Fatalf("second stop: %d", code)
	}
}

func TestIdentityPathValidation(t *testing.T) {
	if _, code := IdentityLoad(""); code != ErrInvalidArg {
		t.Fatalf("empty path: %d", code)
	}
	if _, code := IdentityLoad("bad\x00path"); code != ErrInvalidArg {
		t.Fatalf("nul path: %d", code)
	}
	if _, code := IdentityLoad("/no/such/identity/file"); code != ErrIO {
		t.Fatalf("missing file: got %d want %d err=%q", code, ErrIO, LastError())
	}
}

func TestConfigPathCreate(t *testing.T) {
	node, code := NodeCreate("")
	if code != OK {
		t.Fatalf("empty config: %d %q", code, LastError())
	}
	_ = NodeDestroy(node)

	if _, code := NodeCreate("\x00"); code != ErrInvalidArg {
		t.Fatalf("nul config path: %d", code)
	}
	if _, code := NodeCreate("/no/such/config/file"); code != ErrIO {
		t.Fatalf("missing config: got %d want %d", code, ErrIO)
	}
}

func TestIdentityGenerateDestroy(t *testing.T) {
	id, code := IdentityGenerate()
	if code != OK || id == 0 {
		t.Fatal(code)
	}
	if hex, code := IdentityHashHex(id); code != OK || len(hex) != 32 {
		t.Fatalf("hash: %q code=%d", hex, code)
	}
	if code := IdentityDestroy(id); code != OK {
		t.Fatal(code)
	}
	if code := IdentityDestroy(id); code != ErrInvalidHandle {
		t.Fatalf("double destroy: %d", code)
	}
	if _, code := IdentityHashHex(id); code != ErrInvalidHandle {
		t.Fatalf("use after destroy: %d", code)
	}
}

func TestIdentityLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/identity"
	id, code := IdentityGenerate()
	if code != OK {
		t.Fatal(code)
	}
	rec, err := identityByHandle(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.identity.ToFile(path); err != nil {
		t.Fatal(err)
	}
	_ = IdentityDestroy(id)

	loaded, code := IdentityLoad(path)
	if code != OK {
		t.Fatal(code, LastError())
	}
	t.Cleanup(func() { _ = IdentityDestroy(loaded) })
	hex, code := IdentityHashHex(loaded)
	if code != OK || len(hex) != 32 {
		t.Fatalf("hash %q code %d", hex, code)
	}
}

func TestEventQueueOverflowDropOldest(t *testing.T) {
	q := newEventQueue(2)
	q.push(Event{Kind: EventAnnounce, Hops: 1})
	q.push(Event{Kind: EventAnnounce, Hops: 2})
	q.push(Event{Kind: EventAnnounce, Hops: 3})

	ev, err := q.poll(0)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Hops != 2 {
		t.Fatalf("drop oldest: got hops %d want 2", ev.Hops)
	}
	ev, err = q.poll(0)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Hops != 3 {
		t.Fatalf("remaining: got hops %d want 3", ev.Hops)
	}
}

func TestEventQueueTimeout(t *testing.T) {
	q := newEventQueue(4)
	start := time.Now()
	_, err := q.poll(20 * time.Millisecond)
	if !errors.Is(err, errTimeout) {
		t.Fatalf("got %v want timeout", err)
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Fatal("returned too early")
	}
}

func TestEventQueueClosed(t *testing.T) {
	q := newEventQueue(4)
	q.close()
	_, err := q.poll(0)
	if !errors.Is(err, errState) {
		t.Fatalf("got %v want state", err)
	}
	q.push(Event{Kind: EventAnnounce})
	_, err = q.poll(0)
	if !errors.Is(err, errState) {
		t.Fatalf("push after close still pollable: %v", err)
	}
}

func TestEventQueueConcurrent(t *testing.T) {
	q := newEventQueue(64)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(h uint8) {
			defer wg.Done()
			q.push(Event{Kind: EventAnnounce, Hops: h})
		}(uint8(i))
	}
	wg.Wait()

	seen := make(map[uint8]bool)
	for i := range 32 {
		ev, err := q.poll(0)
		if err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
		seen[ev.Hops] = true
	}
	if len(seen) != 32 {
		t.Fatalf("got %d unique events", len(seen))
	}
}

func TestEventPollTimeoutAndInvalid(t *testing.T) {
	node := mustCreateNode(t)
	_, code := EventPoll(node, 5*time.Millisecond)
	if code != ErrTimeout {
		t.Fatalf("got %d want timeout", code)
	}
	const bogus uint64 = 0xdeadbeef
	_, code = EventPoll(bogus, 0)
	if code != ErrInvalidHandle {
		t.Fatalf("got %d", code)
	}
}

func TestCopyEventField(t *testing.T) {
	src := []byte{1, 2, 3, 4, 5}
	dst := make([]byte, 3)
	n, trunc := CopyEventField(dst, src)
	if n != 3 || !trunc {
		t.Fatalf("n=%d trunc=%v", n, trunc)
	}
	n, trunc = CopyEventField(make([]byte, 8), src)
	if n != 5 || trunc {
		t.Fatalf("full copy n=%d trunc=%v", n, trunc)
	}
	n, trunc = CopyEventField(nil, nil)
	if n != 0 || trunc {
		t.Fatalf("empty n=%d trunc=%v", n, trunc)
	}
}

func TestDecodeHexHash(t *testing.T) {
	_, err := DecodeHexHash("not-hex")
	if !errors.Is(err, errInvalidArg) {
		t.Fatal(err)
	}
	_, err = DecodeHexHash("aabb")
	if !errors.Is(err, errInvalidArg) {
		t.Fatal(err)
	}
	ok := "0123456789abcdef0123456789abcdef"
	b, err := DecodeHexHash(ok)
	if err != nil || len(b) != 16 {
		t.Fatalf("ok decode: %v len=%d", err, len(b))
	}
}

func TestVersion(t *testing.T) {
	if Version() != APIVersion || APIVersion != "1.1" {
		t.Fatalf("version %q", Version())
	}
}

func TestMapErrorWrapped(t *testing.T) {
	code := setLastError(errors.New("wrap: " + errIO.Error()))
	if code != ErrInternal {
		t.Fatalf("non-%%w wrap: got %d want internal", code)
	}
	code = setLastError(fmt.Errorf("%w: disk", errIO))
	if code != ErrIO {
		t.Fatalf("%%w wrap IO: %d", code)
	}
	code = setLastError(errors.Join(errIO, errors.New("disk")))
	if code != ErrIO {
		t.Fatalf("Join IO: %d", code)
	}
	if LastError() == "" {
		t.Fatal("last error empty")
	}
	code = setLastError(nil)
	if code != OK || LastError() != "" {
		t.Fatalf("clear: code=%d err=%q", code, LastError())
	}
}

func TestHandleWrongKind(t *testing.T) {
	node := mustCreateNode(t)
	if _, err := handles.get(node, kindIdentity); !errors.Is(err, errInvalidHandle) {
		t.Fatalf("wrong kind: %v", err)
	}
}

func TestEmptyConfigDisablesShareInstance(t *testing.T) {
	node := mustCreateNode(t)
	rec, err := nodeByHandle(node)
	if err != nil {
		t.Fatal(err)
	}
	if rec.node.Config().ShareInstance {
		t.Fatal("empty config should disable share_instance")
	}
}
