// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux || darwin || freebsd || openbsd || windows) && !js

package interfaces

import (
	"strings"
	"time"

	"go.bug.st/serial"
)

// openHardwareSerial opens a TTY via go.bug.st/serial.
// Covered platforms: Linux (including Android GOOS), macOS, Windows, FreeBSD,
// and OpenBSD. NetBSD and Dragonfly lack complete support in go.bug.st/serial.
func openHardwareSerial(opts SerialOptions) (SerialPort, error) {
	mode := &serial.Mode{
		BaudRate: opts.Speed,
		DataBits: opts.DataBits,
		StopBits: serial.StopBits(opts.StopBits),
	}
	switch strings.ToLower(opts.Parity) {
	case "e", "even":
		mode.Parity = serial.EvenParity
	case "o", "odd":
		mode.Parity = serial.OddParity
	default:
		mode.Parity = serial.NoParity
	}
	p, err := serial.Open(opts.Device, mode)
	if err != nil {
		return nil, err
	}
	_ = p.SetReadTimeout(50 * time.Millisecond)
	return p, nil
}
