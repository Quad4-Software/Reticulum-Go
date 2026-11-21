// SPDX-License-Identifier: LicenseRef-Reticulum
package librrc_test

import (
	"testing"
	"time"

	"github.com/Quad4-Software/reticulum-go-protocols/pkg/librrc"
)

func TestHubClientLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("hub-client loopback skipped in -short mode")
	}

	const (
		hubLocal  = "127.0.0.1:42520"
		hubPeer   = "127.0.0.1:42521"
		cliLocal  = "127.0.0.1:42521"
		cliPeer   = "127.0.0.1:42520"
		pathWait  = 15 * time.Second
		eventWait = 10 * time.Second
	)

	hubNode, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("hub node: %d", code)
	}
	defer librrc.NodeDestroy(hubNode)

	cliNode, code := librrc.NodeCreate("")
	if code != librrc.OK {
		t.Fatalf("client node: %d", code)
	}
	defer librrc.NodeDestroy(cliNode)

	if code := librrc.NodeAddUDPInterface(hubNode, "H1", hubLocal, hubPeer); code != librrc.OK {
		t.Fatalf("hub udp: %d", code)
	}
	if code := librrc.NodeAddUDPInterface(cliNode, "C1", cliLocal, cliPeer); code != librrc.OK {
		t.Fatalf("client udp: %d", code)
	}

	idH, code := librrc.IdentityGenerate()
	if code != librrc.OK {
		t.Fatal(code)
	}
	defer librrc.IdentityDestroy(idH)

	idC, code := librrc.IdentityGenerate()
	if code != librrc.OK {
		t.Fatal(code)
	}
	defer librrc.IdentityDestroy(idC)

	if code := librrc.NodeSetIdentity(hubNode, idH); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.NodeSetIdentity(cliNode, idC); code != librrc.OK {
		t.Fatal(code)
	}

	if code := librrc.NodeStart(hubNode); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.NodeStart(cliNode); code != librrc.OK {
		t.Fatal(code)
	}

	hub, code := librrc.HubCreate(hubNode, idH, "bind-hub", "1.0")
	if code != librrc.OK {
		t.Fatalf("hub create: %d", code)
	}
	defer librrc.HubDestroy(hub)

	if code := librrc.HubStart(hub); code != librrc.OK {
		t.Fatal(code)
	}
	if code := librrc.HubAnnounce(hub); code != librrc.OK {
		t.Fatal(code)
	}

	hubHash, code := librrc.HubHash(hub)
	if code != librrc.OK || len(hubHash) != 16 {
		t.Fatalf("hub hash: %d len=%d", code, len(hubHash))
	}
	if code := librrc.IdentitySeedDestination(idH, hubHash); code != librrc.OK {
		t.Fatal(code)
	}

	deadline := time.Now().Add(pathWait)
	for {
		ok, code := librrc.NodeHasPath(cliNode, hubHash)
		if code == librrc.OK && ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}

	client, code := librrc.ClientDial(cliNode, idC, hubHash, "alice", "bind-test", "1.0", 15000)
	if code != librrc.OK {
		t.Fatalf("dial: %d", code)
	}
	runClientSession(t, hub, client, eventWait)
}

func runClientSession(t *testing.T, hub, client uint64, timeout time.Duration) {
	t.Helper()
	defer librrc.ClientClose(client)

	if code := librrc.ClientJoin(client, "#lobby"); code != librrc.OK {
		t.Fatalf("join: %d", code)
	}

	deadline := time.Now().Add(timeout)
	joined := false
	for time.Now().Before(deadline) {
		ev, code := librrc.ClientEventPoll(client, 500)
		if code == librrc.OK && ev.Kind == librrc.EventJoined {
			joined = true
			break
		}
	}
	if !joined {
		t.Fatal("join timeout")
	}

	want := "hello from binding"
	if code := librrc.ClientSendMsg(client, "#lobby", want); code != librrc.OK {
		t.Fatalf("send: %d", code)
	}

	deadline = time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ev, code := librrc.HubEventPoll(hub, 500)
		if code != librrc.OK {
			continue
		}
		if ev.Kind == librrc.EventMsg && ev.Body == want {
			return
		}
	}
	t.Fatal("hub did not receive message")
}
