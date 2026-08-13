// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package transport

import (
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
