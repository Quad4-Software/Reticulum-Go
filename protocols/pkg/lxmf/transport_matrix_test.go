// SPDX-License-Identifier: LicenseRef-Reticulum
package lxmf

import (
	"fmt"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"net"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
)

// matrixTransport builds an isolated transport for interface-mix tests.
// Relays pass enableTransport so announces and packets forward across
// the chain. Endpoint transports may disable it.
func matrixTransport(t *testing.T, enableTransport bool) *transport.Transport {
	t.Helper()
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.EnableTransport = enableTransport
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func matrixUDP(t *testing.T, tr *transport.Transport, name, local, target string) {
	t.Helper()
	iface, err := interfaces.NewUDPInterface(name, local, target, true)
	if err != nil {
		t.Fatalf("udp %s: %v", name, err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface(name, iface); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("udp start %s: %v", name, err)
	}
	t.Cleanup(func() { _ = iface.Stop() })
}

func matrixTCPServer(t *testing.T, tr *transport.Transport, name string, port int) {
	t.Helper()
	iface, err := interfaces.NewTCPServerInterface(name, "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatalf("tcp server %s: %v", name, err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface(name, iface); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("tcp server start %s: %v", name, err)
	}
	t.Cleanup(func() { _ = iface.Stop() })
}

func matrixTCPClient(t *testing.T, tr *transport.Transport, name string, port int) {
	t.Helper()
	iface, err := interfaces.NewTCPClientInterface(name, "127.0.0.1", port, false, false, true)
	if err != nil {
		t.Fatalf("tcp client %s: %v", name, err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface(name, iface); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("tcp client start %s: %v", name, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !iface.IsOnline() {
		if time.Now().After(deadline) {
			t.Fatalf("tcp client %s offline", name)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = iface.Stop() })
}

// matrixLocalServer hosts a shared instance on hubTr. Spawned clients
// are registered on the hub like upstream's spawn hook.
func matrixLocalServer(t *testing.T, hubTr *transport.Transport, port int) {
	t.Helper()
	spawn := func(client *interfaces.LocalClientInterface) {
		client.In = true
		client.Out = true
		if err := hubTr.RegisterInterface(client.GetName(), client); err != nil {
			t.Errorf("register spawned local client: %v", err)
		}
	}
	srv, err := interfaces.NewLocalServerInterface(port, "", false, spawn, nil)
	if err != nil {
		t.Fatalf("local server: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("local server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
}

// matrixLocalClient attaches tr to the shared instance on port.
func matrixLocalClient(t *testing.T, tr *transport.Transport, port int) {
	t.Helper()
	iface, err := interfaces.NewLocalClientInterface(port, "", false, nil)
	if err != nil {
		t.Fatalf("local client: %v", err)
	}
	iface.In = true
	iface.Out = true
	if err := tr.RegisterInterface(iface.GetName(), iface); err != nil {
		t.Fatalf("register local client: %v", err)
	}
	if err := iface.Start(); err != nil {
		t.Fatalf("local client start: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !iface.IsOnline() {
		if time.Now().After(deadline) {
			t.Fatal("local client offline")
		}
		time.Sleep(20 * time.Millisecond)
	}
	tr.SetConnectedToSharedInstance(true)
	t.Cleanup(func() {
		tr.SetConnectedToSharedInstance(false)
		_ = iface.Stop()
	})
}

// matrixEnd wires an identity, delivery destination, and messenger on tr.
func matrixEnd(t *testing.T, tr *transport.Transport) (*Messenger, *destination.Destination, []byte) {
	t.Helper()
	id := mustNewIdentity(t)
	dest, err := NewDeliveryDestination(id, tr)
	if err != nil {
		t.Fatal(err)
	}
	h := dest.GetHash()
	identity.Remember(nil, h, id.GetPublicKey(), nil)
	return NewMessenger(tr, dest), dest, h
}

// matrixMutualPath announces a and b until both transports hold a path,
// matching the announce/poll pattern in newLXMFMesh.
func matrixMutualPath(t *testing.T, trA, trB *transport.Transport, hashA, hashB []byte, destA, destB *destination.Destination) {
	t.Helper()
	deadline := time.Now().Add(pathEstablishWait)
	lastAnnounce := time.Time{}
	for {
		if trA.HasPath(hashB) && trB.HasPath(hashA) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mutual path timeout: a to b=%v b to a=%v", trA.HasPath(hashB), trB.HasPath(hashA))
		}
		// Stay under the announce burst limit of 8 per 10s.
		if time.Since(lastAnnounce) >= 1500*time.Millisecond {
			_ = destA.Announce(false, nil, nil)
			_ = destB.Announce(false, nil, nil)
			lastAnnounce = time.Now()
		}
		time.Sleep(60 * time.Millisecond)
	}
}

func matrixSendText(t *testing.T, m *Messenger, dst []byte, body string) {
	t.Helper()
	if _, err := m.SendText(dst, "matrix", body); err != nil {
		t.Fatalf("send %q: %v", body, err)
	}
}

func matrixSendDirect(t *testing.T, m *Messenger, dstHash, srcHash []byte, body string) {
	t.Helper()
	msg, err := NewMessage(dstHash, srcHash, []byte("matrix"), []byte(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SendDirect(msg); err != nil {
		t.Fatalf("direct %q: %v", body, err)
	}
}

func matrixExpect(t *testing.T, ch <-chan string, want string) {
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

func matrixHandler(ch chan string) func(*LXMessage, common.NetworkInterface) {
	return func(msg *LXMessage, _ common.NetworkInterface) {
		select {
		case ch <- msg.ContentString():
		default:
		}
	}
}

// matrixUDPLink joins two transports with a point-to-point UDP pair.
func matrixUDPLink(t *testing.T, a, b *transport.Transport, tag string) {
	t.Helper()
	pa, pb := freeUDPPort(t), freeUDPPort(t)
	matrixUDP(t, a, fmt.Sprintf("%s-a", tag), fmt.Sprintf("127.0.0.1:%d", pa), fmt.Sprintf("127.0.0.1:%d", pb))
	matrixUDP(t, b, fmt.Sprintf("%s-b", tag), fmt.Sprintf("127.0.0.1:%d", pb), fmt.Sprintf("127.0.0.1:%d", pa))
}

// matrixTCPLink joins two transports with a TCP server/client pair.
func matrixTCPLink(t *testing.T, server, client *transport.Transport, tag string) {
	t.Helper()
	port := freeUDPPort(t) // a free port is a free port on loopback
	matrixTCPServer(t, server, fmt.Sprintf("%s-srv", tag), port)
	matrixTCPClient(t, client, fmt.Sprintf("%s-cli", tag), port)
}

// matrixUDPChain returns links+1 transports wired in a line. Inner
// nodes relay, endpoints do not.
func matrixUDPChain(t *testing.T, links int) []*transport.Transport {
	t.Helper()
	nodes := make([]*transport.Transport, links+1)
	for i := range nodes {
		nodes[i] = matrixTransport(t, i != 0 && i != len(nodes)-1)
	}
	for i := range links {
		matrixUDPLink(t, nodes[i], nodes[i+1], fmt.Sprintf("hop%d", i))
	}
	return nodes
}

func TestMatrix_DeliveryTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	trA := matrixTransport(t, false)
	trB := matrixTransport(t, false)
	matrixTCPLink(t, trA, trB, "tcp")

	mA, destA, hA := matrixEnd(t, trA)
	mB, destB, hB := matrixEnd(t, trB)
	matrixMutualPath(t, trA, trB, hA, hB, destA, destB)

	gotA := make(chan string, 4)
	gotB := make(chan string, 4)
	mA.SetMessageHandler(matrixHandler(gotA))
	mB.SetMessageHandler(matrixHandler(gotB))

	matrixSendText(t, mA, hB, "tcp-opp-a2b")
	matrixSendText(t, mB, hA, "tcp-opp-b2a")
	matrixExpect(t, gotB, "tcp-opp-a2b")
	matrixExpect(t, gotA, "tcp-opp-b2a")

	matrixSendDirect(t, mA, hB, hA, "tcp-link-a2b")
	matrixSendDirect(t, mB, hA, hB, "tcp-link-b2a")
	matrixExpect(t, gotB, "tcp-link-a2b")
	matrixExpect(t, gotA, "tcp-link-b2a")
}

func TestMatrix_DeliveryMultiHop(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	for _, links := range []int{3, 4} {
		t.Run(fmt.Sprintf("hops=%d", links), func(t *testing.T) {
			nodes := matrixUDPChain(t, links)
			mA, destA, hA := matrixEnd(t, nodes[0])
			mB, destB, hB := matrixEnd(t, nodes[len(nodes)-1])
			matrixMutualPath(t, nodes[0], nodes[len(nodes)-1], hA, hB, destA, destB)

			if hops := int(nodes[0].HopsTo(hB)); hops != links {
				t.Fatalf("a to b hops %d want %d", hops, links)
			}

			gotA := make(chan string, 4)
			gotB := make(chan string, 4)
			mA.SetMessageHandler(matrixHandler(gotA))
			mB.SetMessageHandler(matrixHandler(gotB))

			matrixSendText(t, mA, hB, fmt.Sprintf("hop%d-a2b", links))
			matrixSendText(t, mB, hA, fmt.Sprintf("hop%d-b2a", links))
			matrixExpect(t, gotB, fmt.Sprintf("hop%d-a2b", links))
			matrixExpect(t, gotA, fmt.Sprintf("hop%d-b2a", links))

			matrixSendDirect(t, mA, hB, hA, fmt.Sprintf("link%d-a2b", links))
			matrixExpect(t, gotB, fmt.Sprintf("link%d-a2b", links))
		})
	}
}

func TestMatrix_DeliveryMixedUDPTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	// a --UDP- to relay --TCP- to b
	a := matrixTransport(t, false)
	relay := matrixTransport(t, true)
	b := matrixTransport(t, false)
	matrixUDPLink(t, a, relay, "mix-udp")
	matrixTCPLink(t, relay, b, "mix-tcp")

	mA, destA, hA := matrixEnd(t, a)
	mB, destB, hB := matrixEnd(t, b)
	matrixMutualPath(t, a, b, hA, hB, destA, destB)

	if hops := a.HopsTo(hB); hops != 2 {
		t.Fatalf("a to b hops %d want 2", hops)
	}

	gotA := make(chan string, 4)
	gotB := make(chan string, 4)
	mA.SetMessageHandler(matrixHandler(gotA))
	mB.SetMessageHandler(matrixHandler(gotB))

	matrixSendText(t, mA, hB, "mix-a2b")
	matrixSendText(t, mB, hA, "mix-b2a")
	matrixExpect(t, gotB, "mix-a2b")
	matrixExpect(t, gotA, "mix-b2a")

	matrixSendDirect(t, mA, hB, hA, "mix-link-a2b")
	matrixSendDirect(t, mB, hA, hB, "mix-link-b2a")
	matrixExpect(t, gotB, "mix-link-a2b")
	matrixExpect(t, gotA, "mix-link-b2a")
}

func TestMatrix_DeliverySharedInstance(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	// client --local shared instance- to hub --UDP- to remote
	hub := matrixTransport(t, true)
	remote := matrixTransport(t, false)
	matrixUDPLink(t, hub, remote, "si-udp")

	port := freeUDPPort(t)
	matrixLocalServer(t, hub, port)
	client := matrixTransport(t, false)
	matrixLocalClient(t, client, port)

	mC, destC, hC := matrixEnd(t, client)
	mR, destR, hR := matrixEnd(t, remote)
	matrixMutualPath(t, client, remote, hC, hR, destC, destR)

	gotC := make(chan string, 4)
	gotR := make(chan string, 4)
	mC.SetMessageHandler(matrixHandler(gotC))
	mR.SetMessageHandler(matrixHandler(gotR))

	matrixSendText(t, mC, hR, "si-c2r")
	matrixSendText(t, mR, hC, "si-r2c")
	matrixExpect(t, gotR, "si-c2r")
	matrixExpect(t, gotC, "si-r2c")

	matrixSendDirect(t, mC, hR, hC, "si-link-c2r")
	matrixSendDirect(t, mR, hC, hR, "si-link-r2c")
	matrixExpect(t, gotR, "si-link-c2r")
	matrixExpect(t, gotC, "si-link-r2c")
}

func TestMatrix_DeliveryOnLANAddr(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	var lanIP net.IP
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ni := range ifaces {
			if ni.Flags&net.FlagUp == 0 || ni.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := ni.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
					lanIP = ipn.IP
					break
				}
			}
			if lanIP != nil {
				break
			}
		}
	}
	if lanIP == nil {
		t.Skip("no non-loopback IPv4 address")
	}

	trA := matrixTransport(t, false)
	trB := matrixTransport(t, false)
	pa, pb := freeUDPPort(t), freeUDPPort(t)
	matrixUDP(t, trA, "lan-a", fmt.Sprintf("%s:%d", lanIP, pa), fmt.Sprintf("%s:%d", lanIP, pb))
	matrixUDP(t, trB, "lan-b", fmt.Sprintf("%s:%d", lanIP, pb), fmt.Sprintf("%s:%d", lanIP, pa))

	mA, destA, hA := matrixEnd(t, trA)
	mB, destB, hB := matrixEnd(t, trB)
	matrixMutualPath(t, trA, trB, hA, hB, destA, destB)

	gotA := make(chan string, 4)
	gotB := make(chan string, 4)
	mA.SetMessageHandler(matrixHandler(gotA))
	mB.SetMessageHandler(matrixHandler(gotB))

	matrixSendText(t, mA, hB, "lan-a2b")
	matrixSendText(t, mB, hA, "lan-b2a")
	matrixExpect(t, gotB, "lan-a2b")
	matrixExpect(t, gotA, "lan-b2a")
}

func TestMatrix_UnknownDestinationFails(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh test")
	}
	tr := matrixTransport(t, false)
	peer := matrixTransport(t, false)
	matrixUDPLink(t, tr, peer, "unknown")

	m, _, _ := matrixEnd(t, tr)

	// The peer creates a delivery destination but never announces it.
	// The sender can recall the identity but has no path, so send must
	// issue a path request, wait path_request_wait, then surface the
	// transport error.
	pid := mustNewIdentity(t)
	pdest, err := NewDeliveryDestination(pid, peer)
	if err != nil {
		t.Fatal(err)
	}
	destHash := pdest.GetHash()
	identity.Remember(nil, destHash, pid.GetPublicKey(), nil)

	start := time.Now()
	_, err = m.SendText(destHash, "matrix", "nowhere")
	if err == nil {
		t.Fatal("send to unknown path unexpectedly succeeded")
	}
	if elapsed := time.Since(start); elapsed < PathRequestWait/2 {
		t.Fatalf("send failed in %s, expected a path wait near %s", elapsed, PathRequestWait)
	}
}
