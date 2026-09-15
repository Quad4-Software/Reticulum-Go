// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build (linux || darwin || freebsd || openbsd || windows) && !js

package interfaces

import (
	"time"

	"go.bug.st/serial"
)

func openRNodeSerial(opts SerialOptions) (SerialPort, error) {
	p, err := serial.Open(opts.Device, &serial.Mode{
		BaudRate: rnodeSerialSpeed,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, err
	}
	_ = p.SetReadTimeout(50 * time.Millisecond)
	return p, nil
}
