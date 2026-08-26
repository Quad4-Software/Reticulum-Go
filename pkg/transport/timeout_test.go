// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/packet"
)

func TestFirstHopTimeoutUnknownPathUsesPerHopDefault(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	got := tr.FirstHopTimeout(make([]byte, 16))
	if got != float64(EstablishmentTimeoutPerHop) {
		t.Fatalf("unknown path first hop timeout = %v, want %v", got, EstablishmentTimeoutPerHop)
	}
}

func TestFirstHopTimeoutUsesNextHopBitrate(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	bi := &bitrateIface{}
	bi.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
	bi.Online = true
	bi.bitrate = 125
	if err := tr.RegisterInterface("radio", bi); err != nil {
		t.Fatalf("register: %v", err)
	}
	dest := make([]byte, 16)
	dest[0] = 0x11
	tr.UpdatePath(dest, nil, "radio", 1)

	got := tr.FirstHopTimeout(dest)
	want := float64(packet.MTU)*8/125 + float64(EstablishmentTimeoutPerHop)
	if got != want {
		t.Fatalf("first hop timeout = %v, want %v", got, want)
	}
}

func TestPathResponseWindowColdPathUsesSlowestBitrate(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	slow := &bitrateIface{}
	slow.BaseInterface = interfaces.NewBaseInterface("lora", common.IFTypeUDP, true)
	slow.Online = true
	slow.bitrate = 125
	fast := &bitrateIface{}
	fast.BaseInterface = interfaces.NewBaseInterface("tcp", common.IFTypeTCP, true)
	fast.Online = true
	fast.bitrate = 10_000_000
	if err := tr.RegisterInterface("lora", slow); err != nil {
		t.Fatalf("register slow: %v", err)
	}
	if err := tr.RegisterInterface("tcp", fast); err != nil {
		t.Fatalf("register fast: %v", err)
	}

	got := tr.PathResponseWindow(make([]byte, 16))
	want := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 125)
	if got != want {
		t.Fatalf("path window = %s, want %s", got, want)
	}
	if got <= 15*time.Second {
		t.Fatalf("125 bit/s cold path window %s should exceed the 15s floor", got)
	}
}

func TestPathResponseWindowFiveBitPerSecIsValid(t *testing.T) {
	got := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 5)
	floor50 := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 50)
	if got <= floor50 {
		t.Fatalf("5 bit/s window %s should exceed a 50 bit/s clamp %s", got, floor50)
	}
	air := 2*(float64(PathExchangeBytes)*8/5) + float64(PathWindowMarginSec)
	want := time.Duration(air * float64(time.Second))
	if got != want {
		t.Fatalf("5 bit/s window = %s, want %s", got, want)
	}
}

func TestPathResponseWindowNoBitrateUsesPathRequestTimeout(t *testing.T) {
	got := PathResponseWindowFrom(float64(EstablishmentTimeoutPerHop), 0)
	want := time.Duration(PathRequestTimeout) * time.Second
	if got != want {
		t.Fatalf("no-bitrate window = %s, want %s", got, want)
	}
}

func TestSlowestOnlineBitrateSkipsOffline(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	off := &bitrateIface{}
	off.BaseInterface = interfaces.NewBaseInterface("off", common.IFTypeUDP, true)
	off.Online = false
	off.bitrate = 50
	on := &bitrateIface{}
	on.BaseInterface = interfaces.NewBaseInterface("on", common.IFTypeUDP, true)
	on.Online = true
	on.bitrate = 1200
	if err := tr.RegisterInterface("off", off); err != nil {
		t.Fatalf("register off: %v", err)
	}
	if err := tr.RegisterInterface("on", on); err != nil {
		t.Fatalf("register on: %v", err)
	}
	if got := tr.SlowestOnlineBitrate(); got != 1200 {
		t.Fatalf("slowest = %d, want 1200", got)
	}
}

func TestSlowestOnlineBitrateSkipsReceiveOnly(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	ro := &bitrateIface{}
	ro.BaseInterface = interfaces.NewBaseInterface("rx", common.IFTypeUDP, true)
	ro.Online = true
	ro.bitrate = 50
	ro.SetOutgoingAllowed(false)
	tx := &bitrateIface{}
	tx.BaseInterface = interfaces.NewBaseInterface("tx", common.IFTypeUDP, true)
	tx.Online = true
	tx.bitrate = 1200
	if err := tr.RegisterInterface("rx", ro); err != nil {
		t.Fatalf("register rx: %v", err)
	}
	if err := tr.RegisterInterface("tx", tx); err != nil {
		t.Fatalf("register tx: %v", err)
	}
	if got := tr.SlowestOnlineBitrate(); got != 1200 {
		t.Fatalf("slowest = %d, want 1200", got)
	}
}

func TestDiscoveryTimeoutUsesSlowestOutgoingFanout(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	slow := &bitrateIface{}
	slow.BaseInterface = interfaces.NewBaseInterface("lora", common.IFTypeUDP, true)
	slow.Online = true
	slow.bitrate = 125
	fast := &bitrateIface{}
	fast.BaseInterface = interfaces.NewBaseInterface("tcp", common.IFTypeTCP, true)
	fast.Online = true
	fast.bitrate = 10_000_000
	rx := &bitrateIface{}
	rx.BaseInterface = interfaces.NewBaseInterface("rx", common.IFTypeUDP, true)
	rx.Online = true
	rx.bitrate = 5
	rx.SetOutgoingAllowed(false)
	if err := tr.RegisterInterface("lora", slow); err != nil {
		t.Fatalf("register slow: %v", err)
	}
	if err := tr.RegisterInterface("tcp", fast); err != nil {
		t.Fatalf("register fast: %v", err)
	}
	if err := tr.RegisterInterface("rx", rx); err != nil {
		t.Fatalf("register rx: %v", err)
	}

	got := tr.DiscoveryTimeout(nil)
	want := mediumRoundTripTimeout(125)
	if got != want {
		t.Fatalf("discovery timeout = %s, want %s", got, want)
	}
	if got <= 15*time.Second {
		t.Fatalf("125 bit/s discovery timeout %s should exceed the 15s floor", got)
	}
}

func TestDiscoveryTimeoutFloorWhenNoFanout(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	got := tr.DiscoveryTimeout(nil)
	want := time.Duration(PathRequestTimeout) * time.Second
	if got != want {
		t.Fatalf("empty discovery timeout = %s, want %s", got, want)
	}
}

func TestExtraLinkProofTimeoutOutboundAirtime(t *testing.T) {
	out := &bitrateIface{}
	out.BaseInterface = interfaces.NewBaseInterface("radio", common.IFTypeUDP, true)
	out.Online = true
	out.bitrate = 125
	got := ExtraLinkProofTimeout(out)
	want := time.Duration(float64(packet.MTU) * 8 / 125 * float64(time.Second))
	if got != want {
		t.Fatalf("extra proof timeout = %s, want %s", got, want)
	}
	if ExtraLinkProofTimeout(nil) != 0 {
		t.Fatal("nil iface extra proof timeout should be 0")
	}
}

func TestAwaitPathRespectsCallerDeadline(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	dest := make([]byte, 16)
	dest[0] = 0x22
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := tr.AwaitPath(ctx, dest)
	if err == nil {
		t.Fatal("expected timeout waiting for unknown dest")
	}
	if !errors.Is(err, common.ErrNoPathToDestination) {
		t.Fatalf("AwaitPath = %v, want ErrNoPathToDestination", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("caller deadline should win, elapsed %s", time.Since(start))
	}
}

func TestAwaitPathReturnsWhenPathLearned(t *testing.T) {
	tr := NewTransport(common.DefaultConfig())
	bi := &bitrateIface{}
	bi.BaseInterface = interfaces.NewBaseInterface("udp", common.IFTypeUDP, true)
	bi.Online = true
	bi.bitrate = 1_000_000
	if err := tr.RegisterInterface("udp", bi); err != nil {
		t.Fatalf("register: %v", err)
	}
	dest := make([]byte, 16)
	dest[0] = 0x33
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- tr.AwaitPath(ctx, dest)
	}()
	time.Sleep(50 * time.Millisecond)
	tr.UpdatePath(dest, nil, "udp", 1)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AwaitPath: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AwaitPath did not return after path update")
	}
}

func TestAwaitPathRetryUsesPathRequestMI(t *testing.T) {
	tr := NewTransport(&common.ReticulumConfig{EnableTransport: true})
	defer tr.Close()
	tr.SetIdentity(mustIdentity(t))

	out := newRelayIface("out")
	if err := tr.RegisterInterface("out", out); err != nil {
		t.Fatalf("register: %v", err)
	}

	dest := bytes.Repeat([]byte{0x77}, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- tr.AwaitPath(ctx, dest)
	}()

	select {
	case <-time.After(5 * time.Second):
	case err := <-done:
		if err == nil {
			t.Fatal("unexpected path before timeout")
		}
	}

	if n := len(out.snapshot()); n != 1 {
		t.Fatalf("AwaitPath emitted %d path requests in 5s, want 1 while inside PathRequestMI", n)
	}
}
