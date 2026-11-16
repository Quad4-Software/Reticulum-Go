// SPDX-License-Identifier: LicenseRef-Reticulum
package integration_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"git.quad4.io/reticulum/Reticulum-Go/pkg/common"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/destination"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/identity"
	"git.quad4.io/reticulum/Reticulum-Go/pkg/transport"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/call"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/media"
	"github.com/Quad4-Software/reticulum-go-protocols/pkg/lxst/proto"
)

type simIface struct {
	common.BaseInterface
	peer      *simIface
	inbox     chan []byte
	dropEvery int
	lossPct   int
	delay     time.Duration
	reorder   int
	sent      int
	pending   [][]byte
	mu        sync.Mutex
	rng       *rand.Rand
}

func newSimIface(name string) *simIface {
	s := &simIface{
		BaseInterface: common.NewBaseInterface(name, common.IFTypeAuto, true),
		inbox:         make(chan []byte, 256),
		rng:           rand.New(rand.NewSource(1)),
	}
	s.MTU = common.DefaultMTU
	s.Bitrate = 100000
	s.In = true
	s.Out = true
	s.Enable()
	go func() {
		for data := range s.inbox {
			s.ProcessIncoming(data)
		}
	}()
	return s
}

func (s *simIface) deliver(peer *simIface, data []byte, delay time.Duration) {
	if delay > 0 {
		time.Sleep(delay)
	}
	select {
	case peer.inbox <- data:
	default:
	}
}

func (s *simIface) Send(data []byte, _ string) error {
	if s.peer == nil {
		return nil
	}
	s.mu.Lock()
	s.sent++
	sent := s.sent
	dropEvery := s.dropEvery
	lossPct := s.lossPct
	reorder := s.reorder
	delay := s.delay
	peer := s.peer
	var drop bool
	if dropEvery > 0 && sent%dropEvery == 0 {
		drop = true
	}
	if !drop && lossPct > 0 && s.rng.Intn(100) < lossPct {
		drop = true
	}
	if drop {
		s.mu.Unlock()
		return nil
	}
	cp := append([]byte(nil), data...)
	if reorder > 1 {
		s.pending = append(s.pending, cp)
		if len(s.pending) < reorder {
			s.mu.Unlock()
			return nil
		}
		held := s.pending
		s.pending = nil
		s.mu.Unlock()
		for i, j := 0, len(held)-1; i < j; i, j = i+1, j-1 {
			held[i], held[j] = held[j], held[i]
		}
		for _, p := range held {
			pkt := p
			go s.deliver(peer, pkt, delay)
		}
		return nil
	}
	s.mu.Unlock()
	go s.deliver(peer, cp, delay)
	return nil
}

func (s *simIface) ProcessOutgoing(data []byte) error { return s.Send(data, "") }

func wireSim(a, b *simIface) {
	a.peer = b
	b.peer = a
}

func TestSimCallSignaling(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh integration test")
	}
	cfgA := common.DefaultConfig()
	cfgB := common.DefaultConfig()
	cfgA.ShareInstance = false
	cfgB.ShareInstance = false
	cfgA.InMemoryPathTable = true
	cfgB.InMemoryPathTable = true
	cfgA.InMemoryKnownDestinations = true
	cfgB.InMemoryKnownDestinations = true
	cfgA.ConfigPath = t.TempDir() + "/config"
	cfgB.ConfigPath = t.TempDir() + "/config"
	tA := transport.NewTransport(cfgA)
	tB := transport.NewTransport(cfgB)
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}

	ifA := newSimIface("a")
	ifB := newSimIface("b")
	wireSim(ifA, ifB)
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
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { incoming <- c },
		},
	}, nil)
	sb.Bind(destB)

	_ = destB.Announce(false, nil, nil)
	time.Sleep(100 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()

	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	if caller.State() != call.StateActive {
		t.Fatalf("expected active call, got %v", caller.State())
	}
	_ = caller.Hangup("test done")
}

func TestSimLossyCall(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh integration test")
	}
	cfgA := common.DefaultConfig()
	cfgB := common.DefaultConfig()
	cfgA.ShareInstance = false
	cfgB.ShareInstance = false
	cfgA.InMemoryPathTable = true
	cfgB.InMemoryPathTable = true
	cfgA.InMemoryKnownDestinations = true
	cfgB.InMemoryKnownDestinations = true
	cfgA.ConfigPath = t.TempDir() + "/config"
	cfgB.ConfigPath = t.TempDir() + "/config"
	tA := transport.NewTransport(cfgA)
	tB := transport.NewTransport(cfgB)
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}

	ifA := newSimIface("a")
	ifB := newSimIface("b")
	ifA.dropEvery = 5
	ifB.dropEvery = 5
	wireSim(ifA, ifB)
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
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { incoming <- c },
		},
	}, nil)
	sb.Bind(destB)

	_ = destB.Announce(false, nil, nil)
	time.Sleep(150 * time.Millisecond)

	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()

	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("lossy dial failed: %v", err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if caller.RecvFrames() > 0 && caller.State() == call.StateActive {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if caller.State() != call.StateActive {
		t.Fatalf("expected active call on lossy path, got %v", caller.State())
	}
	if caller.RecvFrames() == 0 {
		t.Fatal("expected media frames on lossy path")
	}
	_ = caller.Hangup("lossy done")
}

func simPair(t *testing.T, tweak func(a, b *simIface)) (*transport.Transport, *transport.Transport, *identity.Identity, *identity.Identity, *simIface, *simIface, chan *call.Call) {
	t.Helper()
	cfgA := common.DefaultConfig()
	cfgB := common.DefaultConfig()
	cfgA.ShareInstance = false
	cfgB.ShareInstance = false
	cfgA.InMemoryPathTable = true
	cfgB.InMemoryPathTable = true
	cfgA.InMemoryKnownDestinations = true
	cfgB.InMemoryKnownDestinations = true
	cfgA.ConfigPath = t.TempDir() + "/config"
	cfgB.ConfigPath = t.TempDir() + "/config"
	tA := transport.NewTransport(cfgA)
	tB := transport.NewTransport(cfgB)
	if err := tA.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tB.Start(); err != nil {
		t.Fatal(err)
	}
	ifA := newSimIface("a")
	ifB := newSimIface("b")
	if tweak != nil {
		tweak(ifA, ifB)
	}
	wireSim(ifA, ifB)
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
	sb := call.NewSwitchboard(tB, call.Config{
		Identity: idB,
		UseAudio: false,
		Events: call.Events{
			OnRinging: func(c *call.Call) { incoming <- c },
		},
	}, nil)
	sb.Bind(destB)
	_ = destB.Announce(false, nil, nil)
	time.Sleep(150 * time.Millisecond)
	return tA, tB, idA, idB, ifA, ifB, incoming
}

func TestSimDelayReorderCadence(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh integration test")
	}
	tA, _, idA, idB, ifA, ifB, incoming := simPair(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	var gaps []time.Duration
	var last time.Time
	var gapMu sync.Mutex
	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnFrame: func(pcm []int16) {
				if len(pcm) == 0 {
					return
				}
				now := time.Now()
				gapMu.Lock()
				if !last.IsZero() {
					gaps = append(gaps, now.Sub(last))
				}
				last = now
				gapMu.Unlock()
			},
		},
	})
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	ifA.mu.Lock()
	ifA.delay = 80 * time.Millisecond
	ifA.mu.Unlock()
	ifB.mu.Lock()
	ifB.delay = 80 * time.Millisecond
	ifB.mu.Unlock()
	time.Sleep(3 * time.Second)
	if caller.State() != call.StateActive {
		t.Fatalf("state %v", caller.State())
	}
	gapMu.Lock()
	cp := append([]time.Duration(nil), gaps...)
	gapMu.Unlock()
	if len(cp) < 10 {
		t.Fatalf("frames %d", len(cp))
	}
	for i := range cp {
		for j := i + 1; j < len(cp); j++ {
			if cp[j] < cp[i] {
				cp[i], cp[j] = cp[j], cp[i]
			}
		}
	}
	mid := cp[len(cp)/2]
	if mid < 40*time.Millisecond || mid > 90*time.Millisecond {
		t.Fatalf("frame p50 %s want near 60ms", mid)
	}
	ifA.mu.Lock()
	ifA.reorder = 2
	ifA.mu.Unlock()
	ifB.mu.Lock()
	ifB.reorder = 2
	ifB.mu.Unlock()
	time.Sleep(time.Second)
	if caller.State() != call.StateActive {
		t.Fatalf("state after reorder %v", caller.State())
	}
	_ = caller.Hangup("done")
}

func TestSimAdaptiveOnLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh integration test")
	}
	tA, _, idA, idB, _, _, incoming := simPair(t, func(a, b *simIface) {
		a.dropEvery = 5
		b.dropEvery = 5
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	var stats []media.LinkMetrics
	var mu sync.Mutex
	caller := call.NewCall(tA, call.Config{
		Identity: idA,
		UseAudio: false,
		Events: call.Events{
			OnStats: func(_ *call.Call, m media.LinkMetrics) {
				mu.Lock()
				stats = append(stats, m)
				mu.Unlock()
			},
		},
	})
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(stats)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	got := append([]media.LinkMetrics(nil), stats...)
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no OnStats")
	}
	if got[len(got)-1].Bitrate <= 0 {
		t.Fatalf("bitrate %d", got[len(got)-1].Bitrate)
	}
	if caller.State() != call.StateActive {
		t.Fatalf("state %v", caller.State())
	}
	if caller.RecvFrames() == 0 {
		t.Fatal("expected media despite loss")
	}
	_ = caller.Hangup("done")
}

func TestSimFivePercentLossRate(t *testing.T) {
	if testing.Short() {
		t.Skip("live mesh integration test")
	}
	tA, _, idA, idB, _, _, incoming := simPair(t, func(a, b *simIface) {
		a.dropEvery = 20
		b.dropEvery = 20
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		select {
		case callee := <-incoming:
			_ = callee.Answer(ctx)
		case <-ctx.Done():
		}
	}()
	caller := call.NewCall(tA, call.Config{Identity: idA, UseAudio: false})
	if err := caller.Dial(ctx, idB); err != nil {
		t.Fatalf("dial: %v", err)
	}
	time.Sleep(3 * time.Second)
	if caller.State() != call.StateActive {
		t.Fatalf("state %v", caller.State())
	}
	fps := float64(caller.RecvFrames()) / 3.0
	if fps < 12 || fps > 18 {
		t.Fatalf("recv fps %f want near 16.67", fps)
	}
	_ = caller.Hangup("done")
}
