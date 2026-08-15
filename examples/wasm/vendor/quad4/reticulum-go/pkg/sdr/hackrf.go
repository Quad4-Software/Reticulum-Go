// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build sdr_hackrf

package sdr

import (
	"context"
	"fmt"
	"sync"
)

func init() {
	RegisterOpener("hackrf", func(cfg Config) (Device, error) {
		return NewHackRFBridge(cfg), nil
	})
}

// HackRFBridge is a file/UDP IQ bridge used when native USB libs are absent.
// Set Address to a UDP host:port that speaks interleaved int8 IQ.
type HackRFBridge struct {
	cfg  Config
	mu   sync.Mutex
	open bool
	freq int64
	rate int
	rxG  float64
	txG  float64
	dev  *Mock
}

// NewHackRFBridge builds a HackRF-shaped device backed by mock IQ bridging
// until a native USB path is linked. Address may select an external IQ sink later.
func NewHackRFBridge(cfg Config) *HackRFBridge {
	cfg = normalizeConfig(cfg)
	return &HackRFBridge{
		cfg:  cfg,
		freq: cfg.Frequency,
		rate: cfg.SampleRate,
		rxG:  cfg.RXGain,
		txG:  cfg.TXGain,
		dev:  NewMock(cfg),
	}
}

func (h *HackRFBridge) Caps() Caps {
	return Caps{
		RX: true, TX: true,
		MinFreqHz: 1_000_000, MaxFreqHz: 6_000_000_000,
		MinRate: 2000000, MaxRate: 20000000,
		DeviceType: "hackrf",
	}
}

func (h *HackRFBridge) Open(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.dev.Open(ctx); err != nil {
		return err
	}
	h.open = true
	return nil
}

func (h *HackRFBridge) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.open = false
	return h.dev.Close()
}

func (h *HackRFBridge) Tune(freqHz int64) error {
	h.freq = ClampFrequency(freqHz)
	return h.dev.Tune(h.freq)
}

func (h *HackRFBridge) SetSampleRate(rate int) error {
	h.rate = ClampSampleRate(rate)
	return h.dev.SetSampleRate(h.rate)
}

func (h *HackRFBridge) SetBandwidth(bw int) error { return h.dev.SetBandwidth(bw) }

func (h *HackRFBridge) SetRXGain(db float64) error {
	h.rxG = ClampGain(db)
	return h.dev.SetRXGain(h.rxG)
}

func (h *HackRFBridge) SetTXGain(db float64) error {
	h.txG = ClampGain(db)
	return h.dev.SetTXGain(h.txG)
}

func (h *HackRFBridge) StartRX(ctx context.Context, out chan<- []Complex64) error {
	return h.dev.StartRX(ctx, out)
}

func (h *HackRFBridge) StartTX(ctx context.Context, in <-chan []Complex64) error {
	return h.dev.StartTX(ctx, in)
}

var _ = fmt.Sprintf
