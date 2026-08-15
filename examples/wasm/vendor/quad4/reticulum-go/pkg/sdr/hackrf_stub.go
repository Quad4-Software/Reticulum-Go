// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !sdr_hackrf

package sdr

import (
	"context"
	"fmt"
)

func init() {
	RegisterOpener("hackrf", func(Config) (Device, error) {
		return nil, fmt.Errorf("hackrf backend requires build tag sdr_hackrf (use device=mock for tests)")
	})
}

// HackRFStub is the default build placeholder.
type HackRFStub struct{}

func (HackRFStub) Caps() Caps                                        { return Caps{DeviceType: "hackrf"} }
func (HackRFStub) Open(context.Context) error                        { return errUnknownDevice }
func (HackRFStub) Close() error                                      { return nil }
func (HackRFStub) Tune(int64) error                                  { return errUnknownDevice }
func (HackRFStub) SetSampleRate(int) error                           { return errUnknownDevice }
func (HackRFStub) SetBandwidth(int) error                            { return errUnknownDevice }
func (HackRFStub) SetRXGain(float64) error                           { return errUnknownDevice }
func (HackRFStub) SetTXGain(float64) error                           { return errUnknownDevice }
func (HackRFStub) StartRX(context.Context, chan<- []Complex64) error { return errUnknownDevice }
func (HackRFStub) StartTX(context.Context, <-chan []Complex64) error { return errUnknownDevice }
