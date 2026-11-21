// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/interfaces"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func matrixTransportCall(t *testing.T, enableTransport bool) *transport.Transport {
	t.Helper()
	cfg := isolatedConfig(t)
	cfg.EnableTransport = enableTransport
	tr := transport.NewTransport(cfg)
	if err := tr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tr.Close() })
	return tr
}

func freeCallPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func callUDP(t *testing.T, tr *transport.Transport, name, local, target string) {
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

func callUDPLink(t *testing.T, a, b *transport.Transport, tag string) {
	t.Helper()
	pa, pb := freeCallPort(t), freeCallPort(t)
	callUDP(t, a, fmt.Sprintf("%s-a", tag), fmt.Sprintf("127.0.0.1:%d", pa), fmt.Sprintf("127.0.0.1:%d", pb))
	callUDP(t, b, fmt.Sprintf("%s-b", tag), fmt.Sprintf("127.0.0.1:%d", pb), fmt.Sprintf("127.0.0.1:%d", pa))
}

func callTCPLink(t *testing.T, server, client *transport.Transport, tag string) {
	t.Helper()
	port := freeCallPort(t)
	srv, err := interfaces.NewTCPServerInterface(tag+"-srv", "127.0.0.1", port, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	srv.In = true
	srv.Out = true
	if err := server.RegisterInterface(srv.GetName(), srv); err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	cli, err := interfaces.NewTCPClientInterface(tag+"-cli", "127.0.0.1", port, false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	cli.In = true
	cli.Out = true
	if err := client.RegisterInterface(cli.GetName(), cli); err != nil {
		t.Fatal(err)
	}
	if err := cli.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !cli.IsOnline() {
		if time.Now().After(deadline) {
			t.Fatal("tcp client offline")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = cli.Stop() })
}

// callChain returns links+1 transports wired in a UDP line; inner
// nodes relay.
func callChain(t *testing.T, links int) []*transport.Transport {
	t.Helper()
	nodes := make([]*transport.Transport, links+1)
	for i := range nodes {
		nodes[i] = matrixTransportCall(t, i != 0 && i != len(nodes)-1)
	}
	for i := 0; i < links; i++ {
		callUDPLink(t, nodes[i], nodes[i+1], fmt.Sprintf("hop%d", i))
	}
	return nodes
}

// waitCallPath announces dest until tr holds a path, staying under the
// announce burst limit.
func waitCallPath(t *testing.T, tr *transport.Transport, dest *destination.Destination, want []byte) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	last := time.Time{}
	for {
		if tr.HasPath(want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("path timeout")
		}
		if time.Since(last) >= 1500*time.Millisecond {
			if err := dest.Announce(false, nil, nil); err != nil {
				t.Logf("announce: %v", err)
			}
			last = time.Now()
		}
		time.Sleep(60 * time.Millisecond)
	}
}

// runCallPair dials idB over the caller transport and asserts answer
// and clean hangup.
func runCallPair(t *testing.T, callerTr, calleeTr *transport.Transport, idA, idB *identity.Identity, destB *destination.Destination) {
	t.Helper()

	ringing := make(chan *call.Call, 1)
	answered := make(chan struct{}, 1)
	sb := call.NewSwitchboard(calleeTr, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, nil)
	sb.Bind(destB)

	caller := call.NewCall(callerTr, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnAnswered: func(*call.Call) {
				select {
				case answered <- struct{}{}:
				default:
				}
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			if err := callee.Answer(ctx); err != nil {
				t.Errorf("answer: %v", err)
			}
		case <-ctx.Done():
		}
	}()

	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	select {
	case <-answered:
	case <-ctx.Done():
		t.Fatal("timeout waiting for answered")
	}
	if err := caller.Hangup("done"); err != nil {
		t.Fatalf("hangup: %v", err)
	}
}

func callEndpoints(t *testing.T, callerTr, calleeTr *transport.Transport) (*identity.Identity, *identity.Identity, *destination.Destination) {
	t.Helper()
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, calleeTr, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	return idA, idB, destB
}

func TestGoGoCallMultiHopUDP(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh call test")
	}
	for _, links := range []int{3} {
		t.Run(fmt.Sprintf("hops=%d", links), func(t *testing.T) {
			nodes := callChain(t, links)
			idA, idB, destB := callEndpoints(t, nodes[0], nodes[len(nodes)-1])
			waitCallPath(t, nodes[0], destB, destB.GetHash())
			if hops := int(nodes[0].HopsTo(destB.GetHash())); hops != links {
				t.Fatalf("path hops %d want %d", hops, links)
			}
			runCallPair(t, nodes[0], nodes[len(nodes)-1], idA, idB, destB)
		})
	}
}

func TestGoGoCallOverTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh call test")
	}
	trA := matrixTransportCall(t, false)
	trB := matrixTransportCall(t, false)
	callTCPLink(t, trA, trB, "tcp")

	idA, idB, destB := callEndpoints(t, trA, trB)
	waitCallPath(t, trA, destB, destB.GetHash())
	runCallPair(t, trA, trB, idA, idB, destB)
}

func TestGoGoCallMixedUDPTCP(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh call test")
	}
	// caller --UDP--> relay --TCP--> callee
	callerTr := matrixTransportCall(t, false)
	relay := matrixTransportCall(t, true)
	calleeTr := matrixTransportCall(t, false)
	callUDPLink(t, callerTr, relay, "mix-udp")
	callTCPLink(t, relay, calleeTr, "mix-tcp")

	idA, idB, destB := callEndpoints(t, callerTr, calleeTr)
	waitCallPath(t, callerTr, destB, destB.GetHash())
	if hops := callerTr.HopsTo(destB.GetHash()); hops != 2 {
		t.Fatalf("path hops %d want 2", hops)
	}
	runCallPair(t, callerTr, calleeTr, idA, idB, destB)
}
