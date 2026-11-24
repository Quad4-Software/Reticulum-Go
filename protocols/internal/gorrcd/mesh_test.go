// SPDX-License-Identifier: LicenseRef-Reticulum
package gorrcd

import (
	"fmt"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"io"
	"log/slog"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/debug"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/rrc"
)

const daemonPathWait = 15 * time.Second

type daemonMesh struct {
	svc           *Service
	hub           *rrc.Hub
	hubHash       []byte
	trH, trA, trB *transport.Transport
	idA, idB      *identity.Identity
	ifaces        []interfaces.Interface
}

func startUDP(t *testing.T, name, local, peer string, tr *transport.Transport) interfaces.Interface {
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

func newDaemonMesh(t *testing.T, basePort int, cfg Config) *daemonMesh {
	t.Helper()
	debug.SetDebugLevel(0)
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
	ifaceH1 := startUDP(t, "dH1-"+tag, addrH, addrA, trH)
	ifaceA := startUDP(t, "dA-"+tag, addrA, addrH, trA)
	ifaceH2 := startUDP(t, "dH2-"+tag, addrH2, addrB, trH)
	ifaceB := startUDP(t, "dB-"+tag, addrB, addrH2, trB)

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

	svc := NewService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.trust.Load(cfg.TrustedIdentities, cfg.BannedIdentities); err != nil {
		t.Fatal(err)
	}
	svc.id = idH
	svc.sender = append([]byte(nil), idH.Hash()...)

	hubDest, err := rrc.NewHubDestination(idH, trH)
	if err != nil {
		t.Fatal(err)
	}
	hub, err := rrc.NewHub(trH, hubDest, rrc.HubConfig{
		Name:              cfg.HubName,
		Version:           Version,
		Limits:            cfg.HubLimits(),
		IncludeMemberList: cfg.IncludeJoinedMemberList,
		Policy:            svc,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.hub = hub
	svc.dest = hubDest
	hub.Start()
	if err := hubDest.Announce(false, nil, nil); err != nil {
		t.Fatalf("announce: %v", err)
	}
	hubHash := hubDest.GetHash()
	identity.Remember(nil, hubHash, idH.GetPublicKey(), nil)
	identity.Remember(nil, idA.Hash(), idA.GetPublicKey(), nil)
	identity.Remember(nil, idB.Hash(), idB.GetPublicKey(), nil)

	deadline := time.Now().Add(daemonPathWait)
	for !trA.HasPath(hubHash) || !trB.HasPath(hubHash) {
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		_ = hubDest.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	m := &daemonMesh{
		svc: svc, hub: hub, hubHash: hubHash,
		trH: trH, trA: trA, trB: trB,
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

func dialDaemon(t *testing.T, m *daemonMesh, which byte, cfg rrc.ClientConfig) *rrc.Client {
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
	if cfg.WelcomeTimeout <= 0 {
		cfg.WelcomeTimeout = 8 * time.Second
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 20 * time.Second
	}
	var last error
	for range 3 {
		c, err := rrc.Dial(tr, id, m.hubHash, cfg)
		if err == nil {
			t.Cleanup(func() { c.Close() })
			return c
		}
		last = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("dial %c: %v", which, last)
	return nil
}
