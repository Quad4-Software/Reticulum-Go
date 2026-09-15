// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !sdr_rtlsdr

package sdr

import (
	"context"
	"fmt"
)

func init() {
	RegisterOpener("rtlsdr", func(Config) (Device, error) {
		return nil, fmt.Errorf("rtlsdr backend requires build tag sdr_rtlsdr (use device=rtltcp or mock)")
	})
}

// RTLSDRStub satisfies the Device name for docs. Not constructed without the tag.
type RTLSDRStub struct{}

func (RTLSDRStub) Caps() Caps                                        { return Caps{DeviceType: "rtlsdr"} }
func (RTLSDRStub) Open(context.Context) error                        { return errUnknownDevice }
func (RTLSDRStub) Close() error                                      { return nil }
func (RTLSDRStub) Tune(int64) error                                  { return errUnknownDevice }
func (RTLSDRStub) SetSampleRate(int) error                           { return errUnknownDevice }
func (RTLSDRStub) SetBandwidth(int) error                            { return errUnknownDevice }
func (RTLSDRStub) SetRXGain(float64) error                           { return errUnknownDevice }
func (RTLSDRStub) SetTXGain(float64) error                           { return errUnknownDevice }
func (RTLSDRStub) StartRX(context.Context, chan<- []Complex64) error { return errUnknownDevice }
func (RTLSDRStub) StartTX(context.Context, <-chan []Complex64) error { return errUnknownDevice }
