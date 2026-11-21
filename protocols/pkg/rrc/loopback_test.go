// SPDX-License-Identifier: LicenseRef-Reticulum
package rrc

import (
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

const (
	rrcUDP1     = "RRC1"
	rrcUDP2     = "RRC2"
	rrcUDP3     = "RRC3"
	rrcLoopback = "127.0.0.1:0"
	rrcPathWait = 15 * time.Second
)

func TestHubClientLoopback(t *testing.T) {
	if testing.Short() {
		t.Skip("loopback rrc test skipped in -short mode")
	}

	cfgH := testconfig.New()
	cfgH.Interfaces = map[string]*common.InterfaceConfig{
		rrcUDP1: {Type: "UDPInterface", Enabled: true, Address: rrcLoopback, TargetHost: rrcLoopback, Name: rrcUDP1},
	}
	cfgA := testconfig.New()
	cfgA.Interfaces = map[string]*common.InterfaceConfig{
		rrcUDP2: {Type: "UDPInterface", Enabled: true, Address: rrcLoopback, TargetHost: rrcLoopback, Name: rrcUDP2},
	}
	cfgB := testconfig.New()
	cfgB.Interfaces = map[string]*common.InterfaceConfig{
		rrcUDP3: {Type: "UDPInterface", Enabled: true, Address: rrcLoopback, TargetHost: rrcLoopback, Name: rrcUDP3},
	}

	trH := transport.NewTransport(cfgH)
	trA := transport.NewTransport(cfgA)
	trB := transport.NewTransport(cfgB)
	if err := trH.Start(); err != nil {
		t.Fatalf("trH: %v", err)
	}
	defer trH.Close()
	if err := trA.Start(); err != nil {
		t.Fatalf("trA: %v", err)
	}
	defer trA.Close()
	if err := trB.Start(); err != nil {
		t.Fatalf("trB: %v", err)
	}
	defer trB.Close()

	addrH := "127.0.0.1:42510"
	addrA := "127.0.0.1:42511"
	addrB := "127.0.0.1:42512"

	startUDP := func(name, local, peer string, tr *transport.Transport) (interfaces.Interface, error) {
		var iface interfaces.Interface
		var err error
		iface, err = interfaces.NewUDPInterface(name, local, peer, true)
		if err != nil {
			return nil, err
		}
		iface.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { tr.HandlePacket(d, ni) })
		if err := iface.Start(); err != nil {
			return nil, err
		}
		if ni, ok := iface.(common.NetworkInterface); ok {
			if err := tr.RegisterInterface(name, ni); err != nil {
				return nil, err
			}
		}
		return iface, nil
	}

	// Hub talks to both clients; clients talk to hub (mesh via pairwise UDP is awkward).
	// Use hub as center: H<->A and H<->B by registering multiple interfaces on hub,
	// or use a shared broadcast-style: A and B both target hub address.
	ifaceH1, err := startUDP(rrcUDP1, addrH, addrA, trH)
	if err != nil {
		t.Fatalf("ifaceH1: %v", err)
	}
	defer ifaceH1.Stop()

	ifaceA, err := startUDP(rrcUDP2, addrA, addrH, trA)
	if err != nil {
		t.Fatalf("ifaceA: %v", err)
	}
	defer ifaceA.Stop()

	// Second hub interface toward B
	var ifaceH2 interfaces.Interface
	ifaceH2, err = interfaces.NewUDPInterface("RRC1b", "127.0.0.1:42513", addrB, true)
	if err != nil {
		t.Fatalf("ifaceH2: %v", err)
	}
	ifaceH2.SetPacketCallback(func(d []byte, ni common.NetworkInterface) { trH.HandlePacket(d, ni) })
	if err := ifaceH2.Start(); err != nil {
		t.Fatalf("ifaceH2 start: %v", err)
	}
	defer ifaceH2.Stop()
	if ni, ok := ifaceH2.(common.NetworkInterface); ok {
		if err := trH.RegisterInterface("RRC1b", ni); err != nil {
			t.Fatalf("register H2: %v", err)
		}
	}

	ifaceB, err := startUDP(rrcUDP3, addrB, "127.0.0.1:42513", trB)
	if err != nil {
		t.Fatalf("ifaceB: %v", err)
	}
	defer ifaceB.Stop()

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
		t.Fatalf("hub dest: %v", err)
	}
	hub, err := NewHub(trH, hubDest, HubConfig{
		Name:              "test-hub",
		Version:           "0.1.0",
		IncludeMemberList: true,
	})
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	hub.Start()
	defer hub.Close()

	if err := hubDest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}

	hubHash := hubDest.GetHash()
	identity.Remember(nil, hubHash, idH.GetPublicKey(), nil)
	identity.Remember(nil, idA.Hash(), idA.GetPublicKey(), nil)
	identity.Remember(nil, idB.Hash(), idB.GetPublicKey(), nil)

	deadline := time.Now().Add(rrcPathWait)
	for {
		if trA.HasPath(hubHash) && trB.HasPath(hubHash) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout to hub")
		}
		time.Sleep(50 * time.Millisecond)
	}

	joinedA := make(chan struct{}, 1)
	joinedB := make(chan struct{}, 1)
	partedA := make(chan struct{}, 1)
	gotMsg := make(chan string, 1)

	clientA, err := Dial(trA, idA, hubHash, ClientConfig{
		Nick:    "alice",
		Name:    "rrc-test",
		Version: "0.1.0",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#lobby" {
					select {
					case joinedA <- struct{}{}:
					default:
					}
				}
			},
			OnParted: func(room string, _ *Envelope) {
				if room == "#lobby" {
					select {
					case partedA <- struct{}{}:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("dial A: %v", err)
	}
	defer clientA.Close()

	clientB, err := Dial(trB, idB, hubHash, ClientConfig{
		Nick: "bob",
		Handlers: ClientHandlers{
			OnJoined: func(room string, _ [][]byte, _ *Envelope) {
				if room == "#lobby" {
					select {
					case joinedB <- struct{}{}:
					default:
					}
				}
			},
			OnMsg: func(env *Envelope) {
				if s, ok := BodyAsString(env.Body); ok {
					select {
					case gotMsg <- s:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("dial B: %v", err)
	}
	defer clientB.Close()

	if err := clientA.Join("#Lobby"); err != nil {
		t.Fatalf("join A: %v", err)
	}
	if err := clientB.Join("#lobby"); err != nil {
		t.Fatalf("join B: %v", err)
	}

	wait := func(ch <-chan struct{}, who string) {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout waiting for JOINED %s", who)
		}
	}
	wait(joinedA, "A")
	wait(joinedB, "B")

	if err := clientA.SendMsg("#lobby", "hello from alice"); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case msg := <-gotMsg:
		if msg != "hello from alice" {
			t.Fatalf("msg = %q", msg)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for forwarded MSG")
	}

	if err := clientA.Part("#lobby"); err != nil {
		t.Fatalf("part: %v", err)
	}
	select {
	case <-partedA:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for PARTED")
	}
}
