// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"fmt"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

type lxmfMesh struct {
	tr1, tr2 *transport.Transport
	m1, m2   *Messenger
	id1, id2 *identity.Identity
	h1, h2   []byte
	ifaces   []interfaces.Interface
}

func startLXMFUDP(t *testing.T, name, local, peer string, tr *transport.Transport) interfaces.Interface {
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

func newLXMFMesh(t *testing.T, basePort int) *lxmfMesh {
	t.Helper()
	cfg1 := common.DefaultConfig()
	cfg2 := common.DefaultConfig()
	tr1 := transport.NewTransport(cfg1)
	tr2 := transport.NewTransport(cfg2)
	if err := tr1.Start(); err != nil {
		t.Fatalf("tr1: %v", err)
	}
	if err := tr2.Start(); err != nil {
		t.Fatalf("tr2: %v", err)
	}

	addr1 := fmt.Sprintf("127.0.0.1:%d", basePort)
	addr2 := fmt.Sprintf("127.0.0.1:%d", basePort+1)
	iface1 := startLXMFUDP(t, fmt.Sprintf("L1-%d", basePort), addr1, addr2, tr1)
	iface2 := startLXMFUDP(t, fmt.Sprintf("L2-%d", basePort), addr2, addr1, tr2)

	id1 := mustNewIdentity(t)
	id2 := mustNewIdentity(t)
	dest1, err := NewDeliveryDestination(id1, tr1)
	if err != nil {
		t.Fatal(err)
	}
	dest2, err := NewDeliveryDestination(id2, tr2)
	if err != nil {
		t.Fatal(err)
	}
	identity.Remember(nil, dest1.GetHash(), id1.GetPublicKey(), nil)
	identity.Remember(nil, dest2.GetHash(), id2.GetPublicKey(), nil)

	m1 := NewMessenger(tr1, dest1)
	m2 := NewMessenger(tr2, dest2)
	h1 := dest1.GetHash()
	h2 := dest2.GetHash()

	_ = dest1.Announce(false, nil, nil)
	_ = dest2.Announce(false, nil, nil)
	deadline := time.Now().Add(pathEstablishWait)
	for {
		if tr1.HasPath(h2) && tr2.HasPath(h1) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		_ = dest1.Announce(false, nil, nil)
		_ = dest2.Announce(false, nil, nil)
		time.Sleep(40 * time.Millisecond)
	}

	mesh := &lxmfMesh{
		tr1: tr1, tr2: tr2,
		m1: m1, m2: m2,
		id1: id1, id2: id2,
		h1: h1, h2: h2,
		ifaces: []interfaces.Interface{iface1, iface2},
	}
	t.Cleanup(func() {
		for _, iface := range mesh.ifaces {
			_ = iface.Stop()
		}
		_ = tr1.Close()
		_ = tr2.Close()
	})
	return mesh
}

func mustPack(t *testing.T, msg *LXMessage, signer Signer) []byte {
	t.Helper()
	raw, err := msg.Pack(signer)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
