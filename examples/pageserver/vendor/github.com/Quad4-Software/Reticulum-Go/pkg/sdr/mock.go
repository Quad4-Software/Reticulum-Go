// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package sdr

import (
	"context"
	"sync"
	"sync/atomic"
)

func init() {
	RegisterOpener("mock", func(cfg Config) (Device, error) {
		return NewMock(cfg), nil
	})
}

// Mock is an in-process IQ device for tests.
type Mock struct {
	cfg Config

	mu     sync.Mutex
	open   bool
	freq   int64
	rate   int
	bw     int
	rxGain float64
	txGain float64

	overruns  atomic.Uint64
	underruns atomic.Uint64

	rxRing chan []Complex64
	peer   *Mock
}

// NewMock builds a mock device with its own RX ring.
func NewMock(cfg Config) *Mock {
	cfg = normalizeConfig(cfg)
	return &Mock{
		cfg:    cfg,
		freq:   cfg.Frequency,
		rate:   cfg.SampleRate,
		bw:     cfg.Bandwidth,
		rxGain: cfg.RXGain,
		txGain: cfg.TXGain,
		rxRing: make(chan []Complex64, cfg.RingSize),
	}
}

// LinkMocks wires a.TX into b.RX and b.TX into a.RX.
func LinkMocks(a, b *Mock) {
	a.mu.Lock()
	b.mu.Lock()
	a.peer = b
	b.peer = a
	a.mu.Unlock()
	b.mu.Unlock()
}

func (m *Mock) Caps() Caps {
	return Caps{
		RX: true, TX: true,
		MinFreqHz: 1, MaxFreqHz: 6_000_000_000,
		MinRate: 250000, MaxRate: 20000000,
		DeviceType: "mock",
	}
}

func (m *Mock) Open(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.open = true
	return nil
}

func (m *Mock) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.open = false
	return nil
}

func (m *Mock) Tune(freqHz int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.freq = ClampFrequency(freqHz)
	return nil
}

func (m *Mock) SetSampleRate(rate int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rate = ClampSampleRate(rate)
	return nil
}

func (m *Mock) SetBandwidth(bw int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bw = bw
	return nil
}

func (m *Mock) SetRXGain(db float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rxGain = ClampGain(db)
	return nil
}

func (m *Mock) SetTXGain(db float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.txGain = ClampGain(db)
	return nil
}

func (m *Mock) StartRX(ctx context.Context, out chan<- []Complex64) error {
	m.mu.Lock()
	if !m.open {
		m.mu.Unlock()
		return errNotOpen
	}
	rx := m.rxRing
	m.mu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-rx:
				if !ok {
					return
				}
				select {
				case out <- block:
				case <-ctx.Done():
					return
				default:
					m.overruns.Add(1)
				}
			}
		}
	}()
	return nil
}

func (m *Mock) StartTX(ctx context.Context, in <-chan []Complex64) error {
	m.mu.Lock()
	if !m.open {
		m.mu.Unlock()
		return errNotOpen
	}
	peer := m.peer
	m.mu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case block, ok := <-in:
				if !ok {
					return
				}
				if peer == nil {
					m.underruns.Add(1)
					continue
				}
				select {
				case peer.rxRing <- block:
				case <-ctx.Done():
					return
				default:
					m.underruns.Add(1)
				}
			}
		}
	}()
	return nil
}

// Overruns returns RX drops from a full consumer.
func (m *Mock) Overruns() uint64 { return m.overruns.Load() }

// Underruns returns TX drops when the peer ring is full or missing.
func (m *Mock) Underruns() uint64 { return m.underruns.Load() }

// Freq returns the tuned frequency.
func (m *Mock) Freq() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.freq
}

// InjectRX pushes IQ into this mock as if received from the air.
func (m *Mock) InjectRX(block []Complex64) {
	select {
	case m.rxRing <- block:
	default:
		m.overruns.Add(1)
	}
}
