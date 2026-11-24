// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"bytes"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"path/filepath"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/msgpack/v5/pkg/msgpack"
)

func TestFetchPropagatedEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("propagation fetch skipped in -short mode")
	}
	mesh := newRouterMesh(t, 43300)
	defer mesh.close()

	// Upstream treats the trailing 32 bytes of stored data as the PN
	// stamp unconditionally, so uploads must carry a stamp. A low target
	// keeps generation fast while exercising the real validation path.
	mesh.recvRouter.cfg.Propagation.PropagationStampCostTarget = 4
	mesh.recvRouter.cfg.Propagation.PropagationStampCostFlexibility = 0
	if err := mesh.recvRouter.EnablePropagation(); err != nil {
		t.Fatal(err)
	}
	pnDest := mesh.recvRouter.PropagationDestination()
	if pnDest == nil {
		t.Fatal("no propagation destination")
	}
	pnHash := pnDest.GetHash()

	_ = pnDest.Announce(false, nil, nil)
	deadline := time.Now().Add(pathEstablishWait)
	for !mesh.sendTr.HasPath(pnHash) {
		if time.Now().After(deadline) {
			t.Fatal("propagation path timeout")
		}
		_ = pnDest.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	// Upload a message destined to the fetching client's delivery hash.
	sendHash := mesh.sendDeliveryHash()
	msg, err := NewMessage(sendHash, sendHash, []byte("title"), []byte("stored on node"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mesh.sendMessenger.SendPropagated(msg, pnHash, 4); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(pathEstablishWait)
	for len(mesh.recvRouter.store.ListForDestination(sendHash)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("propagation store timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}

	received := make(chan *LXMessage, 4)
	mesh.sendMessenger.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		received <- msg
	})
	mesh.sendMessenger.SetReceiveError(func(err error) { t.Logf("recv err: %v", err) })

	n, err := mesh.sendMessenger.FetchPropagated(pnHash, FetchAllMessages, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 fetched message, got %d", n)
	}

	select {
	case got := <-received:
		if !bytes.Equal(got.Content, []byte("stored on node")) {
			t.Fatalf("unexpected content %q", got.Content)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fetched message was not delivered to handler")
	}

	// The follow-up haves request purges the entry on the node.
	deadline = time.Now().Add(pathEstablishWait)
	for len(mesh.recvRouter.store.ListForDestination(sendHash)) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("node did not purge fetched message")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A second fetch lists nothing and returns zero.
	n, err = mesh.sendMessenger.FetchPropagated(pnHash, FetchAllMessages, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 messages on refetch, got %d", n)
	}
}

func TestFetchPropagatedInvalidNode(t *testing.T) {
	if testing.Short() {
		t.Skip("propagation fetch skipped in -short mode")
	}
	m := &Messenger{}
	if _, err := m.FetchPropagated(nil, FetchAllMessages, 0); err == nil {
		t.Fatal("expected error for uninitialized messenger")
	}

	mesh := newLXMFMesh(t, 43310)
	if _, err := mesh.m1.FetchPropagated([]byte{0x01, 0x02}, FetchAllMessages, 0); err == nil {
		t.Fatal("expected error for malformed node hash")
	}
}

func TestMessageGetTransferLimit(t *testing.T) {
	tr := transport.NewTransport(testconfig.New())
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tr.Close() }()

	id := mustNewIdentity(t)
	home := t.TempDir()
	router, err := NewRouter(id, tr, RouterOptions{
		Config:      DefaultConfig(),
		StoragePath: filepath.Join(home, "storage"),
		MessagesDir: filepath.Join(home, "storage", "messages"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.EnablePropagation(); err != nil {
		t.Fatal(err)
	}

	clientID := mustNewIdentity(t)
	delHash, err := deliveryDestinationHash(clientID)
	if err != nil {
		t.Fatal(err)
	}

	mkLXM := func(pad int) []byte {
		data := append([]byte(nil), delHash...)
		return append(data, bytes.Repeat([]byte{0xab}, pad)...)
	}
	stamp := make([]byte, StampSize)
	lxm1 := mkLXM(64)
	lxm2 := mkLXM(256)
	if _, err := router.store.Add(lxm1, stamp, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := router.store.Add(lxm2, stamp, 4); err != nil {
		t.Fatal(err)
	}
	t1 := sha256Sum(lxm1)
	t2 := sha256Sum(lxm2)
	tid1, tid2 := t1[:], t2[:]

	// A third entry owned by a different delivery destination.
	otherLXM := append(bytes.Repeat([]byte{0xcd}, DestinationLength), 0x01)
	if _, err := router.store.Add(otherLXM, stamp, 4); err != nil {
		t.Fatal(err)
	}
	t3 := sha256Sum(otherLXM)
	tid3 := t3[:]

	get := func(req any) any {
		data, err := msgpack.Marshal(req)
		if err != nil {
			t.Fatal(err)
		}
		return router.messageGetRequestHandler(PathMessageGet, data, nil, nil, clientID, time.Now().Unix())
	}

	// List mode returns the ids for the requesting destination only,
	// smallest stored file first.
	list, ok := get([]any{nil, nil}).([][]byte)
	if !ok || len(list) != 2 {
		t.Fatalf("list response %#v", list)
	}
	if !bytes.Equal(list[0], tid1) || !bytes.Equal(list[1], tid2) {
		t.Fatal("list order or contents mismatch")
	}

	// No third field means unlimited.
	resp, ok := get([]any{[]any{tid1, tid2}, []any{}}).([][]byte)
	if !ok || len(resp) != 2 {
		t.Fatalf("unlimited response %#v", resp)
	}
	if !bytes.Equal(resp[0], lxm1) {
		t.Fatalf("stamp not stripped: got %d bytes", len(resp[0]))
	}

	// A literal zero is a zero-byte limit, not unlimited.
	resp, ok = get([]any{[]any{tid1, tid2}, []any{}, 0}).([][]byte)
	if !ok || len(resp) != 0 {
		t.Fatalf("zero limit response %#v", resp)
	}

	// A limit between the two cumulative sizes returns only the first.
	resp, ok = get([]any{[]any{tid1, tid2}, []any{}, 0.2}).([][]byte)
	if !ok || len(resp) != 1 || !bytes.Equal(resp[0], lxm1) {
		t.Fatalf("partial limit response %#v", resp)
	}

	// Numeric strings coerce like Python float(); junk is unlimited.
	resp, ok = get([]any{[]any{tid1, tid2}, []any{}, "0.2"}).([][]byte)
	if !ok || len(resp) != 1 {
		t.Fatalf("string limit response %#v", resp)
	}
	resp, ok = get([]any{[]any{tid1, tid2}, []any{}, "junk"}).([][]byte)
	if !ok || len(resp) != 2 {
		t.Fatalf("junk limit response %#v", resp)
	}

	// Haves purge only entries owned by the requesting destination.
	get([]any{nil, []any{tid1, tid3}})
	if _, ok := router.store.Get(tid1); ok {
		t.Fatal("have purge did not remove tid1")
	}
	if _, ok := router.store.Get(tid3); !ok {
		t.Fatal("have purge removed a foreign entry")
	}
	if _, ok := router.store.Get(tid2); !ok {
		t.Fatal("tid2 should remain stored")
	}
}

func TestResponseErrorCodeForms(t *testing.T) {
	cases := []struct {
		in   any
		want byte
		ok   bool
	}{
		{int(PeerErrorNoIdentity), PeerErrorNoIdentity, true},
		{int64(PeerErrorNoAccess), PeerErrorNoAccess, true},
		{uint8(PeerErrorThrottled), PeerErrorThrottled, true},
		{[]byte{PeerErrorTimeout}, PeerErrorTimeout, true},
		{nil, 0, false},
		{[]any{}, 0, false},
		{true, 0, false},
		{"nope", 0, false},
	}
	for i, c := range cases {
		got, ok := responseErrorCode(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("case %d: got (%v, %v), want (%v, %v)", i, got, ok, c.want, c.ok)
		}
	}
}
