// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

func TestHangupIdle(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	if err := c.Hangup("noop"); err != nil {
		t.Fatal(err)
	}
	if c.State() != call.StateIdle {
		t.Fatalf("state %v", c.State())
	}
}

func TestMuteRequiresActive(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	if err := c.Mute(true); err != call.ErrNotActive {
		t.Fatalf("mute idle: %v", err)
	}
}

func TestAnswerRejectsOutgoing(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	if err := c.Answer(context.Background()); err == nil {
		t.Fatal("answer on idle")
	}
}

func TestDialNilRemote(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	if err := c.Dial(context.Background(), nil); err == nil {
		t.Fatal("nil remote")
	}
}

func TestDialRequiresIdentity(t *testing.T) {
	c := call.NewCall(nil, call.Config{UseAudio: false})
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Dial(context.Background(), id); err != call.ErrNoIdentity {
		t.Fatalf("got %v", err)
	}
}

func TestBusyWhenAlreadyInCall(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
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
	idC, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 1)
	busy := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(150 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("first dial: %v", err)
	}
	if caller.State() != call.StateActive {
		t.Fatal("first call not active")
	}
	if sb.Active() == nil || sb.Active().State() != call.StateActive {
		t.Fatal("switchboard has no active call")
	}

	second := call.NewCall(tA, call.Config{
		Identity:    idC,
		UseAudio:    false,
		ConnectTime: 4 * time.Second,
		WaitTime:    4 * time.Second,
		Events: call.Events{
			OnBusy: func(*call.Call) { busy <- struct{}{} },
		},
	})
	err = second.Dial(ctx, idB)
	if err == nil {
		t.Fatal("second dial should fail while first is active")
	}
	select {
	case <-busy:
	case <-time.After(2 * time.Second):
		t.Fatal("expected busy callback")
	}
	_ = caller.Hangup("done")
}

func TestPhoneDialOccupiesSwitchboard(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
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
	idC, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	destA, err := destination.New(idA, destination.In, destination.Single, proto.AppName, tA, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	ringing := make(chan *call.Call, 1)
	busy := make(chan struct{}, 1)
	sbA := call.NewSwitchboard(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events:   call.Events{OnRinging: func(c *call.Call) { ringing <- c }},
	}, nil)
	sbA.Bind(destA)
	_ = destA.Announce(false, nil, nil)
	phoneB := call.NewPhone(tB, destB, call.Config{Identity: idB, UseAudio: false})
	_ = destB.Announce(false, nil, nil)
	time.Sleep(150 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	outgoing, err := phoneB.Dial(ctx, idA)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if phoneB.Switchboard().Active() != outgoing {
		t.Fatal("outgoing call should occupy switchboard")
	}

	second := call.NewCall(tA, call.Config{
		Identity:    idC,
		UseAudio:    false,
		ConnectTime: 4 * time.Second,
		WaitTime:    4 * time.Second,
		Events: call.Events{
			OnBusy: func(*call.Call) { busy <- struct{}{} },
		},
	})
	if err := second.Dial(ctx, idB); err == nil {
		t.Fatal("incoming should be busy while outgoing is active")
	}
	select {
	case <-busy:
	case <-time.After(2 * time.Second):
		t.Fatal("expected busy callback")
	}
	_ = outgoing.Hangup("done")
}

func TestMuteDuringCall(t *testing.T) {
	tA := transport.NewTransport(isolatedConfig(t))
	tB := transport.NewTransport(isolatedConfig(t))
	_ = tA.Start()
	_ = tB.Start()
	ifA := newPairIface("a")
	ifB := newPairIface("b")
	ifA.peer = ifB
	ifB.peer = ifA
	_ = tA.RegisterInterface("a", ifA)
	_ = tB.RegisterInterface("b", ifB)
	idA, _ := identity.New()
	idB, _ := identity.New()
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
	time.Sleep(100 * time.Millisecond)
	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatal(err)
	}
	if err := caller.Mute(true); err != nil {
		t.Fatal(err)
	}
	if err := caller.Mute(false); err != nil {
		t.Fatal(err)
	}
	_ = caller.Hangup("done")
}

func TestConcurrentHangup(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	var wg sync.WaitGroup
	wg.Add(8)
	for range 8 {
		go func() {
			defer wg.Done()
			_ = c.Hangup("race")
		}()
	}
	wg.Wait()
}
