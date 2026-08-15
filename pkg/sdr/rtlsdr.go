// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build sdr_rtlsdr

package sdr

import (
	"context"
	"fmt"
)

func init() {
	RegisterOpener("rtlsdr", func(cfg Config) (Device, error) {
		// USB RTL-SDR without CGO uses rtl_tcp on localhost when Address is set.
		// Operators can also point device=rtltcp directly.
		if cfg.Address == "" {
			cfg.Address = "127.0.0.1:1234"
		}
		cfg.Device = "rtltcp"
		return NewRTLTCP(cfg), nil
	})
}

// Ensure the tagged build references Device construction.
var _ = fmt.Sprintf
var _ Device = (*RTLTCP)(nil)
var _ = context.Background
