// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package librns

import (
	"bytes"
	"testing"
	"time"

	"quad4/msgpack/v5/pkg/msgpack"
	"quad4/reticulum-go/pkg/identity"
)

func TestFacadeLinkOpenSendClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping facade link integration in short mode")
	}

	nodeA := mustCreateNode(t)
	nodeB := mustCreateNode(t)
	idA := mustIdentity(t)
	idB := mustIdentity(t)

	if code := NodeSetIdentity(nodeA, idA); code != OK {
		t.Fatal(code, LastError())
	}
	if code := NodeSetIdentity(nodeB, idB); code != OK {
		t.Fatal(code, LastError())
	}

	attachPipePair(t, nodeA, nodeB)

	if code := NodeStart(nodeA); code != OK {
		t.Fatal(code, LastError())
	}
	if code := NodeStart(nodeB); code != OK {
		t.Fatal(code, LastError())
	}
	t.Cleanup(func() {
		_ = NodeStop(nodeA)
		_ = NodeStop(nodeB)
	})

	destA, code := DestinationCreate(nodeA, 0, "librns-facade", []string{"svc"}, true)
	if code != OK {
		t.Fatalf("DestinationCreate: %d %q", code, LastError())
	}
	t.Cleanup(func() { _ = DestinationDestroy(destA) })

	if code := DestinationAnnounce(destA, []byte("hello-app")); code != OK {
		t.Fatal(code, LastError())
	}

	deadline := time.Now().Add(3 * time.Second)
	var announce Event
	for time.Now().Before(deadline) {
		ev, code := EventPoll(nodeB, 50*time.Millisecond)
		if code == ErrTimeout {
			continue
		}
		if code != OK {
			t.Fatalf("EventPoll: %d %q", code, LastError())
		}
		if ev.Kind == EventAnnounce {
			announce = ev
			break
		}
	}
	if announce.Kind != EventAnnounce {
		t.Fatal("timed out waiting for announce on node B")
	}
	if !bytes.Equal(announce.AppData, []byte("hello-app")) {
		t.Fatalf("app data: %q", announce.AppData)
	}

	destHash, code := DestinationHash(destA)
	if code != OK {
		t.Fatal(code)
	}
	if !bytes.Equal(announce.DestinationHash, destHash) {
		t.Fatalf("announce hash mismatch")
	}

	linkB, code := LinkOpen(nodeB, destHash)
	if code != OK {
		t.Fatalf("LinkOpen: %d %q", code, LastError())
	}

	var establishedB, establishedA bool
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !(establishedA && establishedB) {
		if !establishedB {
			ev, code := EventPoll(nodeB, 50*time.Millisecond)
			if code == OK && ev.Kind == EventLinkEstablished {
				establishedB = true
			}
		}
		if !establishedA {
			ev, code := EventPoll(nodeA, 50*time.Millisecond)
			if code == OK && ev.Kind == EventLinkEstablished {
				establishedA = true
			}
		}
	}
	if !establishedB || !establishedA {
		t.Fatalf("link establish: A=%v B=%v", establishedA, establishedB)
	}

	payload := []byte("facade-payload")
	if code := LinkSend(linkB, payload); code != OK {
		t.Fatalf("LinkSend: %d %q", code, LastError())
	}

	deadline = time.Now().Add(5 * time.Second)
	var gotData bool
	for time.Now().Before(deadline) {
		ev, code := EventPoll(nodeA, 50*time.Millisecond)
		if code == ErrTimeout {
			continue
		}
		if code != OK {
			t.Fatal(code, LastError())
		}
		if ev.Kind == EventLinkData {
			if !bytes.Equal(ev.AppData, payload) {
				t.Fatalf("data mismatch: %q", ev.AppData)
			}
			gotData = true
			break
		}
	}
	if !gotData {
		t.Fatal("timed out waiting for link data")
	}

	if code := LinkClose(linkB); code != OK {
		t.Fatal(code, LastError())
	}

	deadline = time.Now().Add(3 * time.Second)
	var closed bool
	for time.Now().Before(deadline) {
		ev, code := EventPoll(nodeA, 50*time.Millisecond)
		if code == ErrTimeout {
			continue
		}
		if code == OK && ev.Kind == EventLinkClosed {
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("timed out waiting for link closed")
	}
}

func TestLinkOpenUnknownDestination(t *testing.T) {
	if testing.Short() {
		t.Skip("AwaitPath window wait")
	}
	node := mustCreateNode(t)
	id := mustIdentity(t)
	if code := NodeSetIdentity(node, id); code != OK {
		t.Fatal(code)
	}
	if code := NodeStart(node); code != OK {
		t.Fatal(code)
	}
	t.Cleanup(func() { _ = NodeStop(node) })
	mustRegisterOutgoingIface(t, node, "link-open-unknown")

	fake := make([]byte, identity.TruncatedHashLength/8)
	for i := range fake {
		fake[i] = byte(i + 1)
	}
	_, code := LinkOpen(node, fake)
	if code != ErrNotFound {
		t.Fatalf("got %d want %d err=%q", code, ErrNotFound, LastError())
	}
	ev, code := EventPoll(node, 50*time.Millisecond)
	if code != OK {
		t.Fatalf("expected link failed event: %d", code)
	}
	if ev.Kind != EventLinkFailed {
		t.Fatalf("kind %d", ev.Kind)
	}
}

func TestLinkOpenBeforeStart(t *testing.T) {
	node := mustCreateNode(t)
	hash := make([]byte, identity.TruncatedHashLength/8)
	_, code := LinkOpen(node, hash)
	if code != ErrState {
		t.Fatalf("got %d want %d", code, ErrState)
	}
}

func TestPathRequestValidation(t *testing.T) {
	node := mustCreateNode(t)
	if code := PathRequest(node, []byte{1, 2}); code != ErrInvalidArg {
		t.Fatalf("short hash: %d", code)
	}
	hash := make([]byte, identity.TruncatedHashLength/8)
	if code := PathRequest(node, hash); code != ErrState {
		t.Fatalf("before start: %d", code)
	}
	if code := NodeStart(node); code != OK {
		t.Fatal(code)
	}
	t.Cleanup(func() { _ = NodeStop(node) })
	mustRegisterOutgoingIface(t, node, "path-req")
	if code := PathRequest(node, hash); code != OK {
		t.Fatal(code, LastError())
	}
}

func TestDestinationCreateRequiresIdentity(t *testing.T) {
	node := mustCreateNode(t)
	_, code := DestinationCreate(node, 0, "app", nil, false)
	if code != ErrState {
		t.Fatalf("got %d want %d", code, ErrState)
	}
}

func TestDestinationCreateEmptyApp(t *testing.T) {
	node := mustCreateNode(t)
	id := mustIdentity(t)
	if code := NodeSetIdentity(node, id); code != OK {
		t.Fatal(code)
	}
	_, code := DestinationCreate(node, 0, "", nil, false)
	if code != ErrInvalidArg {
		t.Fatalf("got %d want %d", code, ErrInvalidArg)
	}
}

func TestFacadeNodeDestinationAnnounce(t *testing.T) {
	nodeHandle := mustCreateNode(t)
	idHandle := mustIdentity(t)

	if code := NodeSetIdentity(nodeHandle, idHandle); code != OK {
		t.Fatal(code)
	}
	if code := NodeStart(nodeHandle); code != OK {
		t.Fatal(code)
	}
	t.Cleanup(func() { _ = NodeStop(nodeHandle) })

	rec, err := nodeByHandle(nodeHandle)
	if err != nil {
		t.Fatal(err)
	}
	sink := newPipeInterface("announce-sink")
	if err := rec.node.Transport().RegisterInterface(sink.GetName(), sink); err != nil {
		t.Fatal(err)
	}

	destHandle, code := DestinationCreate(nodeHandle, 0, "librns-ann", []string{"test"}, true)
	if code != OK {
		t.Fatalf("dest create: %d %q", code, LastError())
	}
	t.Cleanup(func() { _ = DestinationDestroy(destHandle) })

	if code := DestinationEnableRatchets(destHandle, ""); code != OK {
		t.Fatal(code, LastError())
	}
	if code := DestinationEnforceRatchets(destHandle); code != OK {
		t.Fatal(code, LastError())
	}

	if code := DestinationAnnounce(destHandle, []byte("appdata")); code != OK {
		t.Fatal(code, LastError())
	}

	hash, code := DestinationHash(destHandle)
	if code != OK || len(hash) != identity.TruncatedHashLength/8 {
		t.Fatalf("hash len %d code %d", len(hash), code)
	}
}

func TestDecodeLinkRequestPayloadMap(t *testing.T) {
	packed, err := msgpack.Marshal(map[string]any{
		"var_name":   "alice",
		"field_user": "bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeLinkRequestPayload(packed)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("want map[string]any, got %T", got)
	}
	if m["var_name"] != "alice" || m["field_user"] != "bob" {
		t.Fatalf("unexpected map %#v", m)
	}
}

func TestDecodeLinkRequestPayloadRawBytes(t *testing.T) {
	raw := []byte{0x01, 0x02, 0x03}
	got := decodeLinkRequestPayload(raw)
	b, ok := got.([]byte)
	if !ok || !bytes.Equal(b, raw) {
		t.Fatalf("want raw bytes, got %T %#v", got, got)
	}
	if decodeLinkRequestPayload(nil) != nil {
		t.Fatal("nil payload should stay nil")
	}
}
