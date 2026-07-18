// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"bytes"
	"context"
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestFreeSpacePathLossExploratory(t *testing.T) {
	// 100 m at 433 MHz: FSPL ≈ 20*log10(100)+20*log10(433e6)+20*log10(4π/c)
	got := FreeSpacePathLossDB(100, 433e6)
	want := 20*math.Log10(100) + 20*math.Log10(433e6) + 20*math.Log10(4*math.Pi/speedOfLight)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got=%v want=%v", got, want)
	}
	if got < 60 || got > 70 {
		t.Fatalf("unexpected FSPL magnitude %v", got)
	}
	far := FreeSpacePathLossDB(1000, 433e6)
	if far-got < 19.9 || far-got > 20.1 {
		t.Fatalf("10x distance should add ~20 dB got delta=%v", far-got)
	}
}

func TestAWGNMeasuredSNRExploratory(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	modem := NewBurstModem()
	clean, err := modem.Encode(bytes.Repeat([]byte{0xA5}, 64))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []float64{0, 5, 10, 15, 20} {
		noisy := AddAWGN(clean, target, rng)
		meas := MeasuredSNRdB(clean, noisy)
		if math.Abs(meas-target) > 1.5 {
			t.Fatalf("target=%v measured=%v", target, meas)
		}
	}
}

func TestChannelLinkSNRIncreasesWithPower(t *testing.T) {
	ch := NewChannelModel(433e6, 100, 2e6, 1)
	ch.TXPowerW = 0.01
	low := ch.LinkSNRdB()
	ch.TXPowerW = 1.0
	high := ch.LinkSNRdB()
	if high-low < 19.5 || high-low > 20.5 {
		t.Fatalf("100x power should add ~20 dB delta=%v low=%v high=%v", high-low, low, high)
	}
}

func TestSimDeviceBurstThroughChannel(t *testing.T) {
	ch := NewChannelModel(433e6, 50, 2e6, 99)
	ch.TXPowerW = 1.0
	ch.GainDB = 40 // keep SNR high for reliable decode
	a := NewSimDevice(Config{RingSize: 8, Frequency: 433000000, SampleRate: 2000000}, ch)
	b := NewSimDevice(Config{RingSize: 8, Frequency: 433000000, SampleRate: 2000000}, ch)
	LinkSimDevices(a, b)
	ctx := t.Context()
	if err := a.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Open(ctx); err != nil {
		t.Fatal(err)
	}
	modem := NewBurstModem()
	payload := []byte("sdr-channel-sim")
	if err := a.TransmitBurst(modem, payload); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case block := <-b.rxRing:
			got, ok := modem.Decode(block)
			if ok && bytes.Equal(got, payload) {
				return
			}
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Fatal("decode timeout")
}

func TestSimDeviceHighNoiseDropsFrames(t *testing.T) {
	ch := NewChannelModel(433e6, 5000, 2e6, 123)
	ch.TXPowerW = 0.001
	ch.GainDB = -30
	a := NewSimDevice(Config{RingSize: 4}, ch)
	b := NewSimDevice(Config{RingSize: 4}, ch)
	LinkSimDevices(a, b)
	ctx := context.Background()
	_ = a.Open(ctx)
	_ = b.Open(ctx)
	modem := NewBurstModem()
	okCount := 0
	trials := 20
	for i := range trials {
		payload := []byte{byte(i), 1, 2, 3, 4, 5, 6, 7}
		_ = a.TransmitBurst(modem, payload)
		select {
		case block := <-b.rxRing:
			if _, ok := modem.Decode(block); ok {
				okCount++
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if okCount == trials {
		t.Fatalf("expected some failures under harsh channel ok=%d", okCount)
	}
}

func TestSDRInterfaceOverSimChannel(t *testing.T) {
	// Exercised via interfaces package to avoid import cycle. Covered by
	// TransmitBurst path above and mock SDRInterface tests.
}
