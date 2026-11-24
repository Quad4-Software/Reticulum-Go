// SPDX-License-Identifier: LicenseRef-Reticulum
package e2e_test

import (
	"context"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/internal/testconfig"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/call"
	"git.quad4.io/reticulum/Reticulum-Go/protocols/pkg/lxst/proto"
)

type pairIface struct {
	common.BaseInterface
	peer *pairIface
}

func newPairIface(name string) *pairIface {
	p := &pairIface{BaseInterface: common.NewBaseInterface(name, common.IFTypeAuto, true)}
	p.MTU = common.DefaultMTU
	p.Bitrate = 1_000_000
	p.In = true
	p.Out = true
	p.Enable()
	return p
}

func (p *pairIface) Send(data []byte, _ string) error {
	if p.peer == nil {
		return nil
	}
	cp := append([]byte(nil), data...)
	go p.peer.ProcessIncoming(cp)
	return nil
}

func (p *pairIface) ProcessOutgoing(data []byte) error { return p.Send(data, "") }

func TestE2EBothSidesReceiveMedia(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh e2e test")
	}
	cfg := func() *common.ReticulumConfig {
		c := testconfig.New()
		c.ShareInstance = false
		c.InMemoryPathTable = true
		c.InMemoryKnownDestinations = true
		c.ConfigPath = t.TempDir() + "/config"
		return c
	}
	tA := transport.NewTransport(cfg())
	tB := transport.NewTransport(cfg())
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	if err := tA.RegisterInterface("a", ifA); err != nil {
		t.Fatal(err)
	}
	if err := tB.RegisterInterface("b", ifB); err != nil {
		t.Fatal(err)
	}
	idA, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	idB, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events:   call.Events{OnRinging: func(c *call.Call) { ringing <- c }},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(120 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	answered := make(chan *call.Call, 1)
	go func() {
		select {
		case c := <-ringing:
			_ = c.Answer(ctx)
			answered <- c
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	var callee *call.Call
	select {
	case callee = <-answered:
	case <-ctx.Done():
		t.Fatal("callee missing")
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if caller.RecvFrames() > 0 && callee.RecvFrames() > 0 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if caller.RecvFrames() == 0 {
		t.Fatalf("caller received no media caller_sent=%d", caller.SentFrames())
	}
	if callee.SentFrames() == 0 {
		t.Fatal("callee sent no media")
	}
	if caller.SentFrames() == 0 {
		t.Fatal("caller sent no media")
	}
	if callee.RecvFrames() == 0 {
		t.Fatalf("callee received no media caller_sent=%d callee_sent=%d", caller.SentFrames(), callee.SentFrames())
	}
	if caller.State() != call.StateActive || callee.State() != call.StateActive {
		t.Fatalf("states caller=%v callee=%v", caller.State(), callee.State())
	}
	_ = caller.Hangup("e2e")
}
