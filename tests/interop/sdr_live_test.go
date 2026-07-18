// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Live SDR interface tests. Set RUN_LIVE_SDR=1.
// Lab and testing only. Prefer mock. Live TX is not authorized by these tests.
// Mock path always runs. Hardware paths skip when devices are absent.

//go:build !js

package interop

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/interfaces"
	"quad4/reticulum-go/pkg/sdr"
)

func TestLiveSDRMockExchange(t *testing.T) {
	if os.Getenv("RUN_LIVE_SDR") != "1" {
		t.Skip("set RUN_LIVE_SDR=1 to run live SDR tests")
	}

	aDev := sdr.NewMock(sdr.Config{RingSize: 16})
	bDev := sdr.NewMock(sdr.Config{RingSize: 16})
	sdr.LinkMocks(aDev, bDev)

	a, err := interfaces.NewSDRInterface("sdr-a", true, interfaces.SDROptions{
		Device: "mock", Modem: "burst", DeviceOverride: aDev, Bitrate: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := interfaces.NewSDRInterface("sdr-b", true, interfaces.SDROptions{
		Device: "mock", Modem: "burst", DeviceOverride: bDev, Bitrate: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}

	var got []byte
	var mu sync.Mutex
	b.SetPacketCallback(func(data []byte, _ common.NetworkInterface) {
		mu.Lock()
		got = append([]byte(nil), data...)
		mu.Unlock()
	})

	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	defer a.Detach()
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	defer b.Detach()

	payload := []byte("live-sdr-mock")
	if err := a.ProcessOutgoing(payload); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := bytes.Equal(got, payload)
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("got=%q", got)
}

func TestLiveSDRHardwareOpen(t *testing.T) {
	if os.Getenv("RUN_LIVE_SDR") != "1" {
		t.Skip("set RUN_LIVE_SDR=1 to run live SDR tests")
	}
	devType := os.Getenv("SDR_DEVICE")
	if devType == "" {
		t.Skip("set SDR_DEVICE=rtltcp|rtlsdr|hackrf for hardware open probe")
	}
	addr := os.Getenv("SDR_ADDRESS")
	si, err := interfaces.NewSDRInterface("sdr-hw", true, interfaces.SDROptions{
		Device: devType, Address: addr, Modem: "burst",
		Frequency: 433000000, SampleRate: 2000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := si.Start(); err != nil {
		t.Skipf("hardware open failed: %v", err)
	}
	defer si.Detach()
	if !si.IsOnline() {
		t.Fatal("not online")
	}
}
