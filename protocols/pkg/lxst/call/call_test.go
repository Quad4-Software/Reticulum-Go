// SPDX-License-Identifier: LicenseRef-Reticulum
package call_test

import (
	"context"
	"github.com/Quad4-Software/reticulum-go-protocols/internal/testconfig"
	"sync"
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

func isolatedConfig(t *testing.T) *common.ReticulumConfig {
	t.Helper()
	cfg := testconfig.New()
	cfg.ShareInstance = false
	cfg.InMemoryPathTable = true
	cfg.InMemoryKnownDestinations = true
	cfg.ConfigPath = t.TempDir() + "/config"
	return cfg
}

func TestDefaultConfig(t *testing.T) {
	cfg := call.DefaultConfig()
	if cfg.AppName != proto.AppName || cfg.AspectName != proto.AspectName {
		t.Fatalf("unexpected app %s aspect %s", cfg.AppName, cfg.AspectName)
	}
	if cfg.Profile != proto.DefaultProfile {
		t.Fatalf("unexpected profile %d", cfg.Profile)
	}
}

func TestCallStateIdle(t *testing.T) {
	c := call.NewCall(nil, call.DefaultConfig())
	if c.State() != call.StateIdle {
		t.Fatalf("expected idle")
	}
	if c.State().String() != "idle" {
		t.Fatalf("state name %q", c.State().String())
	}
	if call.StateActive.String() != "active" {
		t.Fatal("active name")
	}
}

func TestGoGoCallOverPairedInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh call test")
	}
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
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)

	incoming := make(chan *call.Call, 1)
	ringing := make(chan *call.Call, 1)
	answered := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, func(c *call.Call) {
		incoming <- c
	})
	_ = incoming
	sb.Bind(destB)
	if err := destB.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	var held []int16
	var snapshot int16
	var heldMu sync.Mutex
	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				heldMu.Lock()
				if held == nil {
					held = append([]int16(nil), pcm...)
					snapshot = pcm[0]
				}
				heldMu.Unlock()
			},
			OnAnswered: func(*call.Call) {
				select {
				case answered <- struct{}{}:
				default:
				}
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if caller.RecvFrames() > 0 && caller.State() == call.StateActive {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if caller.State() != call.StateActive {
		t.Fatalf("caller state %d", caller.State())
	}
	if caller.RecvFrames() == 0 {
		t.Fatal("caller received no media frames")
	}
	pcmDeadline := time.Now().Add(time.Second)
	for time.Now().Before(pcmDeadline) {
		heldMu.Lock()
		gotHeld, gotSnap := held, snapshot
		heldMu.Unlock()
		if gotHeld != nil {
			if gotHeld[0] != gotSnap {
				t.Fatal("OnFrame buffer was overwritten after callback returned")
			}
			_ = caller.Hangup("done")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no pcm captured")
}

func TestFirstMediaSoonAfterActive(t *testing.T) {
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
	time.Sleep(80 * time.Millisecond)

	first := make(chan time.Time, 1)
	answeredAt := make(chan time.Time, 1)
	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnAnswered: func(*call.Call) {
				select {
				case answeredAt <- time.Now():
				default:
				}
			},
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				select {
				case first <- time.Now():
				default:
				}
			},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		select {
		case c := <-ringing:
			_ = c.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	var tf, tActive time.Time
	gotFrame, gotAnswered := false, false
	for !gotFrame || !gotAnswered {
		select {
		case tf = <-first:
			gotFrame = true
		case tActive = <-answeredAt:
			gotAnswered = true
		case <-ctx.Done():
			if !gotAnswered {
				t.Fatal("answered timestamp missing")
			}
			t.Fatal("no first frame")
		}
	}
	dt := tf.Sub(tActive)
	if dt > 40*time.Millisecond {
		t.Fatalf("first frame %s after active, want under 40ms", dt)
	}
	_ = caller.Hangup("done")
}

func TestGoGoHalfDuplexCall(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh half duplex call test")
	}
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
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Mode:     proto.ModeHalfDuplex,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(150 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Mode:     proto.ModeHalfDuplex,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			_ = callee.Answer(ctx)
			time.Sleep(400 * time.Millisecond)
			callee.PTT(true)
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if caller.SentFrames() != 0 {
		t.Fatalf("expected no TX while squelched, got %d", caller.SentFrames())
	}
	caller.PTT(true)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && caller.SentFrames() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if caller.SentFrames() == 0 {
		t.Fatal("caller sent no frames after PTT")
	}
	if caller.RecvFrames() == 0 {
		t.Fatal("caller received no frames from callee PTT")
	}
	_ = caller.Hangup("done")
}

func TestGoGoCodec2Call(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh codec2 call test")
	}
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
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Profile:  proto.ProfileBandwidthLow,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, nil)
	sb.Bind(destB)
	if err := destB.Announce(false, nil, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Profile:  proto.ProfileBandwidthLow,
	})
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
		t.Fatalf("codec2 dial: %v", err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if caller.RecvFrames() > 0 && caller.State() == call.StateActive {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if caller.State() != call.StateActive || caller.RecvFrames() == 0 {
		t.Fatalf("codec2 call state=%d frames=%d", caller.State(), caller.RecvFrames())
	}
	_ = caller.Hangup("done")
}

func TestRejectIncoming(t *testing.T) {
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
	rejected := make(chan struct{}, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{
		Identity:    idA,
		UseAudio:    false,
		ConnectTime: 8 * time.Second,
		WaitTime:    8 * time.Second,
		Events: call.Events{
			OnRejected: func(*call.Call) { rejected <- struct{}{} },
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-ringing:
			time.Sleep(200 * time.Millisecond)
			_ = callee.Reject("no")
		case <-ctx.Done():
		}
	}()
	err = caller.Dial(ctx, idB)
	if err == nil {
		t.Fatal("expected dial to fail after reject")
	}
	select {
	case <-rejected:
	case <-time.After(2 * time.Second):
		t.Fatal("expected rejected callback")
	}
}

func TestSequentialCallsAfterHangup(t *testing.T) {
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
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 2)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events:   call.Events{OnRinging: func(c *call.Call) { ringing <- c }},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(80 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		for {
			select {
			case c := <-ringing:
				_ = c.Answer(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("first dial: %v", err)
	}
	occupied := sb.Active()
	_ = caller.Hangup("done")
	if occupied != nil {
		_ = occupied.Hangup("done")
	}
	if sb.Active() != nil {
		t.Fatal("switchboard still occupied after hangup")
	}

	second := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	if err := second.Dial(ctx, idB); err != nil {
		t.Fatalf("second dial: %v", err)
	}
	if second.State() != call.StateActive {
		t.Fatalf("second call state %v", second.State())
	}
	_ = second.Hangup("done")
	if cur := sb.Active(); cur != nil {
		_ = cur.Hangup("done")
	}
}

func TestCalleeFirstFrameSoonAfterEstablished(t *testing.T) {
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
	destB, err := destination.New(idB, destination.In, destination.Single, proto.AppName, tB, proto.AspectName)
	if err != nil {
		t.Fatal(err)
	}
	destB.AcceptsLinks(true)
	ringing := make(chan *call.Call, 1)
	first := make(chan time.Time, 1)
	establishedAt := make(chan time.Time, 1)
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { ringing <- c },
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				select {
				case first <- time.Now():
				default:
				}
			},
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(80 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		select {
		case c := <-ringing:
			_ = c.Answer(ctx)
			select {
			case establishedAt <- time.Now():
			default:
			}
		case <-ctx.Done():
		}
	}()
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	var tf, tEst time.Time
	gotFrame, gotEst := false, false
	for !gotFrame || !gotEst {
		select {
		case tf = <-first:
			gotFrame = true
		case tEst = <-establishedAt:
			gotEst = true
		case <-ctx.Done():
			if !gotEst {
				t.Fatal("answer timestamp missing")
			}
			t.Fatal("no callee first frame")
		}
	}
	dt := tf.Sub(tEst)
	if dt > 40*time.Millisecond {
		t.Fatalf("callee first frame %s after established, want under 40ms", dt)
	}
	if dt >= 10*time.Millisecond {
		t.Fatalf("callee first frame %s waited on receive ticker", dt)
	}
	_ = caller.Hangup("done")
}
