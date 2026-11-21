// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"fmt"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"strings"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/link"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

type testMesh struct {
	trH, trA, trB *transport.Transport
	hub           *Hub
	hubHash       []byte
	idA, idB      *identity.Identity
	ifaces        []interfaces.Interface
}

func startTestUDP(t testing.TB, name, local, peer string, tr *transport.Transport) interfaces.Interface {
	t.Helper()
	var iface interfaces.Interface
	var err error
	iface, err = interfaces.NewUDPInterface(name, local, peer, true)
	if err != nil {
		t.Fatalf("udp %s: %v", name, err)
	}
	iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
	if err := iface.Start(); err != nil {
		t.Fatalf("udp start %s: %v", name, err)
	}
	if ni, ok := iface.(common.NetworkInterface); ok {
		if err := tr.RegisterInterface(name, ni); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	return iface
}

func newTestMesh(t testing.TB, basePort int, hubCfg HubConfig) *testMesh {
	t.Helper()
	cfgH := testconfig.New()
	cfgA := testconfig.New()
	cfgB := testconfig.New()
	trH := transport.NewTransport(cfgH)
	trA := transport.NewTransport(cfgA)
	trB := transport.NewTransport(cfgB)
	for _, tr := range []*transport.Transport{trH, trA, trB} {
		if err := tr.Start(); err != nil {
			t.Fatalf("transport start: %v", err)
		}
	}

	addrH := fmt.Sprintf("127.0.0.1:%d", basePort)
	addrA := fmt.Sprintf("127.0.0.1:%d", basePort+1)
	addrB := fmt.Sprintf("127.0.0.1:%d", basePort+2)
	addrH2 := fmt.Sprintf("127.0.0.1:%d", basePort+3)

	tag := fmt.Sprintf("%d", basePort)
	ifaceH1 := startTestUDP(t, "H1-"+tag, addrH, addrA, trH)
	ifaceA := startTestUDP(t, "A-"+tag, addrA, addrH, trA)
	ifaceH2 := startTestUDP(t, "H2-"+tag, addrH2, addrB, trH)
	ifaceB := startTestUDP(t, "B-"+tag, addrB, addrH2, trB)

	idH, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	idA, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	hubDest, err := NewHubDestination(idH, trH)
	if err != nil {
		t.Fatal(err)
	}
	hub, err := NewHub(trH, hubDest, hubCfg)
	if err != nil {
		t.Fatal(err)
	}
	hub.Start()
	if err := hubDest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	hubHash := hubDest.GetHash()
	identity.Remember(nil, hubHash, idH.GetPublicKey(), nil)

	deadline := time.Now().Add(rrcPathWait)
	for {
		if trA.HasPath(hubHash) && trB.HasPath(hubHash) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		_ = hubDest.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	m := &testMesh{
		trH: trH, trA: trA, trB: trB,
		hub: hub, hubHash: hubHash,
		idA: idA, idB: idB,
		ifaces: []interfaces.Interface{ifaceH1, ifaceA, ifaceH2, ifaceB},
	}
	t.Cleanup(func() {
		hub.Close()
		for _, iface := range m.ifaces {
			_ = iface.Stop()
		}
		_ = trH.Close()
		_ = trA.Close()
		_ = trB.Close()
	})
	return m
}

func dialMeshClient(t testing.TB, m *testMesh, which byte, cfg ClientConfig) *Client {
	t.Helper()
	var tr *transport.Transport
	var id *identity.Identity
	switch which {
	case 'A':
		tr, id = m.trA, m.idA
	case 'B':
		tr, id = m.trB, m.idB
	default:
		t.Fatalf("unknown client %q", which)
	}
	c, err := Dial(tr, id, m.hubHash, cfg)
	if err != nil {
		t.Fatalf("dial %c: %v", which, err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func waitJoined(t testing.TB, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for JOINED %s", label)
	}
}

func mustEnvelope(t testing.TB, typ uint64, sender []byte) *Envelope {
	t.Helper()
	env, err := NewEnvelope(typ, sender)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func dialMeshPreHello(t testing.TB, m *testMesh, which byte, onMsg MessageHandler) *session {
	t.Helper()
	var tr *transport.Transport
	var id *identity.Identity
	switch which {
	case 'A':
		tr, id = m.trA, m.idA
	case 'B':
		tr, id = m.trB, m.idB
	default:
		t.Fatalf("unknown client %q", which)
	}
	remote, err := identity.Recall(m.hubHash)
	if err != nil || remote == nil {
		t.Fatal(err)
	}
	destOut, err := destination.FromHash(m.hubHash, remote, destination.Single, tr)
	if err != nil {
		t.Fatal(err)
	}
	lnk := link.NewLink(destOut, tr, nil, nil, nil)
	sess := newSession(lnk, id.Hash(), true, onMsg, nil)
	if err := lnk.Establish(); err != nil {
		t.Fatal(err)
	}
	lnk.Start()
	deadline := time.Now().Add(10 * time.Second)
	for !lnk.IsActive() {
		if time.Now().After(deadline) {
			t.Fatal("link timeout")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err := lnk.Identify(id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	t.Cleanup(func() { lnk.Teardown() })
	return sess
}

func waitError(t *testing.T, ch <-chan string, substr string) {
	t.Helper()
	select {
	case msg := <-ch:
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(substr)) {
			t.Fatalf("error=%q want substring %q", msg, substr)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for error containing %q", substr)
	}
}
