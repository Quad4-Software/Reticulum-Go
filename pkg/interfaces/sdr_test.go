// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !rns_slim

package interfaces

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"quad4/reticulum-go/pkg/common"
	"quad4/reticulum-go/pkg/sdr"
)

func TestSDRInterfaceMockExchange(t *testing.T) {
	aDev := sdr.NewMock(sdr.Config{RingSize: 16, Frequency: 433000000, SampleRate: 2000000})
	bDev := sdr.NewMock(sdr.Config{RingSize: 16, Frequency: 433000000, SampleRate: 2000000})
	sdr.LinkMocks(aDev, bDev)

	a, err := NewSDRInterface("a", true, SDROptions{
		Device: "mock", Modem: "burst", Bitrate: 1200, DeviceOverride: aDev,
		Frequency: 433000000, SampleRate: 2000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSDRInterface("b", true, SDROptions{
		Device: "mock", Modem: "burst", Bitrate: 1200, DeviceOverride: bDev,
		Frequency: 433000000, SampleRate: 2000000,
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

	payload := []byte("sdr-iface-ping")
	if err := a.ProcessOutgoing(payload); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
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
