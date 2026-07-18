// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestBurstModemRoundTrip(t *testing.T) {
	m := NewBurstModem()
	payload := []byte("reticulum-sdr-burst")
	iq, err := m.Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := m.Decode(iq)
	if !ok || !bytes.Equal(got, payload) {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestBurstModemRejectsGarbage(t *testing.T) {
	m := NewBurstModem()
	iq := make([]Complex64, 256)
	if _, ok := m.Decode(iq); ok {
		t.Fatal("expected reject")
	}
}

func TestBurstModemOffsetPreamble(t *testing.T) {
	m := NewBurstModem()
	payload := []byte("offset-burst")
	iq, err := m.Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	pad := make([]Complex64, m.spb()*3)
	for i := range pad {
		pad[i] = Complex64{I: 0.25, Q: 0}
	}
	noisy := append(pad, iq...)
	got, ok := m.Decode(noisy)
	if !ok || !bytes.Equal(got, payload) {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func BenchmarkBurstModemRoundTrip(b *testing.B) {
	m := NewBurstModem()
	payload := bytes.Repeat([]byte{0x5A}, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iq, err := m.Encode(payload)
		if err != nil {
			b.Fatal(err)
		}
		got, ok := m.Decode(iq)
		if !ok || len(got) != len(payload) {
			b.Fatal("decode")
		}
	}
}

func BenchmarkBurstModemDecodeNoisy(b *testing.B) {
	m := NewBurstModem()
	payload := bytes.Repeat([]byte{0xA5}, 64)
	iq, err := m.Encode(payload)
	if err != nil {
		b.Fatal(err)
	}
	noisy := make([]Complex64, len(iq)+512)
	copy(noisy[512:], iq)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, ok := m.Decode(noisy)
		if !ok || len(got) != len(payload) {
			b.Fatal("decode")
		}
	}
}

func TestMockLinkRoundTrip(t *testing.T) {
	a := NewMock(Config{RingSize: 8})
	b := NewMock(Config{RingSize: 8})
	LinkMocks(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.Open(ctx); err != nil {
		t.Fatal(err)
	}
	rx := make(chan []Complex64, 4)
	tx := make(chan []Complex64, 4)
	if err := b.StartRX(ctx, rx); err != nil {
		t.Fatal(err)
	}
	if err := a.StartTX(ctx, tx); err != nil {
		t.Fatal(err)
	}
	block := []Complex64{{I: 1, Q: 0}, {I: -1, Q: 0}}
	tx <- block
	select {
	case got := <-rx:
		if len(got) != 2 || got[0].I != 1 {
			t.Fatalf("got=%v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestClampHelpers(t *testing.T) {
	if ClampGain(-1) != 0 || ClampGain(100) != 60 {
		t.Fatal("gain")
	}
	if ClampSampleRate(1) != 250000 {
		t.Fatal("rate")
	}
	if ClampFrequency(-5) != 0 {
		t.Fatal("freq")
	}
}

func TestOpenMock(t *testing.T) {
	dev, err := Open(Config{Device: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if err := dev.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = dev.Close()
}

func TestOpenRTLSDRStub(t *testing.T) {
	_, err := Open(Config{Device: "rtlsdr"})
	if err == nil {
		t.Fatal("expected stub error without tag path returning opener error")
	}
}
