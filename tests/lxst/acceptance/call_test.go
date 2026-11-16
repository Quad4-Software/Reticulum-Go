// SPDX-License-Identifier: LicenseRef-Reticulum
package acceptance_test

import (
	"context"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
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

func TestAcceptanceFullCallAndHangup(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh acceptance test")
	}
	cfg := func() *common.ReticulumConfig {
		c := common.DefaultConfig()
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
	ended := make(chan string, 2)
	ringing := make(chan *call.Call, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
			OnEnded:   func(_ *call.Call, reason string) { ended <- reason },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(120 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnEnded: func(_ *call.Call, reason string) { ended <- reason },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			if !callee.Incoming() {
				t.Error("callee not incoming")
				return
			}
			if err := callee.Answer(ctx); err != nil {
				t.Errorf("answer: %v", err)
			}
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	if caller.State() != call.StateActive {
		t.Fatalf("state %v", caller.State())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && caller.RecvFrames() == 0 {
		time.Sleep(40 * time.Millisecond)
	}
	if caller.RecvFrames() == 0 {
		t.Fatal("no media frames")
	}
	if err := caller.Hangup("acceptance hangup"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("missing ended callback")
	}
}
