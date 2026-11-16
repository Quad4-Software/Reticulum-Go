// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
)

func TestE2E_TwoWayMessengerExchange(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}

	mesh := newLXMFMesh(t, 42730)
	got1 := make(chan string, 1)
	got2 := make(chan string, 1)

	mesh.m1.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case got1 <- msg.ContentString():
		default:
		}
	})
	mesh.m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case got2 <- msg.ContentString():
		default:
		}
	})

	if _, err := mesh.m1.SendText(mesh.h2, "e2e", "hello-from-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mesh.m2.SendText(mesh.h1, "e2e", "hello-from-2"); err != nil {
		t.Fatal(err)
	}

	waitBody := func(ch <-chan string, want string) {
		t.Helper()
		select {
		case got := <-ch:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(15 * time.Second):
			t.Fatalf("timeout waiting for %q", want)
		}
	}
	waitBody(got2, "hello-from-1")
	waitBody(got1, "hello-from-2")
}

func TestE2E_FieldsRoundTripOverWire(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}

	mesh := newLXMFMesh(t, 42740)
	got := make(chan *LXMessage, 1)
	mesh.m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case got <- msg:
		default:
		}
	})

	msg, err := mesh.m1.Compose(mesh.h2, "subj", "body-fields", map[byte]any{
		FieldRenderer: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mesh.m1.Send(msg); err != nil {
		t.Fatal(err)
	}

	select {
	case inbound := <-got:
		if inbound.ContentString() != "body-fields" {
			t.Fatalf("content=%q", inbound.ContentString())
		}
		if inbound.Fields[FieldRenderer] != "markdown" {
			t.Fatalf("fields=%#v", inbound.Fields)
		}
		if !inbound.SignatureValidated {
			t.Fatal("signature not validated")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout")
	}
}

func TestE2E_ParallelInboundHandlers(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e skipped in -short")
	}

	mesh := newLXMFMesh(t, 42750)
	var mu sync.Mutex
	count := 0
	mesh.m2.SetMessageHandler(func(msg *LXMessage, _ common.NetworkInterface) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	const n = 5
	for i := range n {
		if _, err := mesh.m1.SendText(mesh.h2, "p", strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := count
		mu.Unlock()
		if c >= n {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("received %d/%d", count, n)
}
