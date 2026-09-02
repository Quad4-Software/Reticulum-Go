// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

//go:build !js

package interfaces

import (
	"io"
	"sync/atomic"
	"time"
)

const (
	serialHWMTU            = 564
	serialDefaultSpeed     = 9600
	serialDefaultDataBits  = 8
	serialDefaultStopBits  = 1
	serialDefaultFrameIdle = 100 * time.Millisecond
	serialDefaultReconnect = 5 * time.Second
	serialReadChunk        = 4096
	serialDefaultIFACSize  = 8
)

// SerialPort is the byte stream behind SerialInterface and RNode transports.
// Tests inject pipes or PTYs. Production opens use go.bug.st/serial where
// available (Linux including Android, macOS, Windows, FreeBSD, OpenBSD).
type SerialPort interface {
	io.ReadWriteCloser
}

// SerialOpenFunc opens a serial device from options.
type SerialOpenFunc func(opts SerialOptions) (SerialPort, error)

// SerialOptions configures SerialInterface beyond the Python baseline.
type SerialOptions struct {
	Device            string
	Speed             int
	DataBits          int
	Parity            string
	StopBits          int
	RTSCTS            bool
	DSRDTR            bool
	XONXOFF           bool
	FrameIdle         time.Duration
	ReconnectDelay    time.Duration
	MaxReconnectTries int
	MTU               int
	Bitrate           int64
	Open              SerialOpenFunc
}

// SerialStats is atomic traffic and health counters for operators.
type SerialStats struct {
	FramesRX      atomic.Uint64
	FramesTX      atomic.Uint64
	BytesRX       atomic.Uint64
	BytesTX       atomic.Uint64
	FramingErrors atomic.Uint64
	Reconnects    atomic.Uint64
	OpenFailures  atomic.Uint64
}
