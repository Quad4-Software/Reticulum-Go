// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

// Package sdr provides SDR front ends and a Go burst modem for SDRInterface.
//
// Lab and testing use. Prefer mock and the math channel for development.
// Live TX is not authorized by this package. Follow local radio rules.
package sdr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
)

// Complex64 is one IQ sample.
type Complex64 struct {
	I float32
	Q float32
}

// Caps describes device capabilities.
type Caps struct {
	RX         bool
	TX         bool
	MinFreqHz  int64
	MaxFreqHz  int64
	MinRate    int
	MaxRate    int
	DeviceType string
}

// Device is a pluggable SDR front end.
type Device interface {
	Caps() Caps
	Open(ctx context.Context) error
	Close() error
	Tune(freqHz int64) error
	SetSampleRate(rate int) error
	SetBandwidth(bw int) error
	SetRXGain(db float64) error
	SetTXGain(db float64) error
	StartRX(ctx context.Context, out chan<- []Complex64) error
	StartTX(ctx context.Context, in <-chan []Complex64) error
}

// Config selects and tunes a device.
type Config struct {
	Device     string
	Serial     string
	Address    string
	Frequency  int64
	SampleRate int
	Bandwidth  int
	RXGain     float64
	TXGain     float64
	RingSize   int
}

var (
	errUnknownDevice = errors.New("unknown sdr device type")
	errRXOnly        = errors.New("device is receive-only")
	errNotOpen       = errors.New("device not open")
)

var (
	openersMu sync.RWMutex
	openers   = map[string]func(Config) (Device, error){}
)

// RegisterOpener registers a device backend by type name.
func RegisterOpener(name string, fn func(Config) (Device, error)) {
	if name == "" || fn == nil {
		return
	}
	openersMu.Lock()
	openers[strings.ToLower(name)] = fn
	openersMu.Unlock()
}

// Open creates a device from cfg.
func Open(cfg Config) (Device, error) {
	cfg = normalizeConfig(cfg)
	openersMu.RLock()
	fn := openers[strings.ToLower(cfg.Device)]
	openersMu.RUnlock()
	if fn == nil {
		return nil, fmt.Errorf("%w: %s", errUnknownDevice, cfg.Device)
	}
	return fn(cfg)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Device == "" {
		cfg.Device = "mock"
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 2000000
	}
	if cfg.Frequency <= 0 {
		cfg.Frequency = 433000000
	}
	if cfg.RingSize <= 0 {
		cfg.RingSize = 32
	}
	cfg.Device = strings.ToLower(strings.TrimSpace(cfg.Device))
	return cfg
}

// ClampGain keeps gain in a simple [0, 60] dB window for config fuzzing.
func ClampGain(db float64) float64 {
	if db < 0 {
		return 0
	}
	if db > 60 {
		return 60
	}
	return db
}

// ClampSampleRate keeps sample rate in a sane window.
func ClampSampleRate(rate int) int {
	if rate < 250000 {
		return 250000
	}
	if rate > 20000000 {
		return 20000000
	}
	return rate
}

// ClampFrequency keeps frequency in the rtl_tcp uint32 command range.
func ClampFrequency(hz int64) int64 {
	if hz < 0 {
		return 0
	}
	if hz > math.MaxUint32 {
		return math.MaxUint32
	}
	return hz
}
